package server

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const redisOperationTimeout = 750 * time.Millisecond
const redisCheckpointClaimTTL = 30 * time.Second

type redisSyncBackend struct {
	client                  *respClient
	prefix                  string
	maxEntries, maxBytes    int
	ttl, checkpointInterval time.Duration
}

type redisEntry struct {
	Fingerprint                                 string            `json:"fingerprint"`
	Fleet                                       string            `json:"fleet"`
	Hashes                                      map[string]string `json:"hashes"`
	ReleaseRef                                  string            `json:"releaseRef"`
	Digest                                      string            `json:"digest"`
	Response                                    syncResponse      `json:"response"`
	ValidUntil                                  int64             `json:"validUntil"`
	CheckpointAt                                int64             `json:"checkpointAt"`
	WindowStart                                 int64             `json:"windowStart"`
	SizeBytes                                   int               `json:"sizeBytes"`
	Global, FleetGeneration, EndpointGeneration uint64
}

func newRedisSyncBackend(config FastPathConfig) (*redisSyncBackend, error) {
	client, err := newRESPClient(config.RedisURL)
	if err != nil {
		return nil, err
	}
	maxEntries, maxBytes := config.MaxEntries, config.MaxBytes
	if maxEntries <= 0 {
		maxEntries = 10_000
	}
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	ttl, checkpoint := config.TTL, config.CheckpointInterval
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if checkpoint <= 0 {
		checkpoint = 5 * time.Minute
	}
	return &redisSyncBackend{client: client, prefix: config.RedisPrefix, maxEntries: maxEntries, maxBytes: maxBytes, ttl: ttl, checkpointInterval: checkpoint}, nil
}

func redisHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (r *redisSyncBackend) key(parts ...string) string {
	return r.prefix + ":usf:v1:" + strings.Join(parts, ":")
}
func (r *redisSyncBackend) endpointKey(id string) string { return r.key("decision", redisHash(id)) }
func (r *redisSyncBackend) scopeKeys(endpointID, fleet string) []string {
	return []string{r.key("generation", "global"), r.key("unstable", "global"), r.key("generation", "fleet", redisHash(fleet)), r.key("unstable", "fleet", redisHash(fleet)), r.key("generation", "endpoint", redisHash(endpointID)), r.key("unstable", "endpoint", redisHash(endpointID))}
}

func (r *redisSyncBackend) snapshot(endpointID, fleet string) (authoritySnapshot, error) {
	keys := r.scopeKeys(endpointID, fleet)
	args := append([]string{"MGET"}, keys...)
	v, err := r.client.command(args...)
	if err != nil {
		return authoritySnapshot{}, err
	}
	items, ok := v.([]any)
	if !ok || len(items) != 6 {
		return authoritySnapshot{}, errors.New("invalid redis generation response")
	}
	n := func(v any) uint64 {
		if v == nil {
			return 0
		}
		parsed, _ := strconv.ParseUint(asString(v), 10, 64)
		return parsed
	}
	return authoritySnapshot{global: n(items[0]), fleet: n(items[2]), endpoint: n(items[4]), stable: n(items[1]) == 0 && n(items[3]) == 0 && n(items[5]) == 0}, nil
}

const redisLookupScript = `
local raw=redis.call('GET',KEYS[1]); if not raw then return {0} end
if redis.call('GET',KEYS[2])~=ARGV[1] and not (redis.call('GET',KEYS[2])==false and ARGV[1]=='0') then return {0} end
if redis.call('EXISTS',KEYS[3])==1 then return {0} end
if redis.call('GET',KEYS[4])~=ARGV[2] and not (redis.call('GET',KEYS[4])==false and ARGV[2]=='0') then return {0} end
if redis.call('EXISTS',KEYS[5])==1 then return {0} end
if redis.call('GET',KEYS[6])~=ARGV[3] and not (redis.call('GET',KEYS[6])==false and ARGV[3]=='0') then return {0} end
if redis.call('EXISTS',KEYS[7])==1 then return {0} end
if tonumber(ARGV[4])>=tonumber(ARGV[5]) then
  if redis.call('SET',KEYS[10],raw,'NX','PX',ARGV[7]) then return {2,raw,redis.call('HINCRBY',KEYS[9],KEYS[1],1)} end
  return {0}
end
redis.call('ZADD',KEYS[8],ARGV[4],KEYS[1]); return {1,raw,redis.call('HINCRBY',KEYS[9],KEYS[1],1)}
`

func (r *redisSyncBackend) get(endpointID, fingerprint string, request syncRequest, now time.Time) (syncResponse, bool, *syncCheckpoint, error) {
	key := r.endpointKey(endpointID)
	// Decode first only to supply the expected generation tuple to the atomic lookup.
	rawValue, err := r.client.command("GET", key)
	if err != nil || rawValue == nil {
		return syncResponse{}, false, nil, err
	}
	var probe redisEntry
	if json.Unmarshal([]byte(asString(rawValue)), &probe) != nil {
		_, _ = r.client.command("DEL", key)
		return syncResponse{}, false, nil, nil
	}
	keys := append([]string{key}, r.scopeKeys(endpointID, probe.Fleet)...)
	keys = append(keys, r.key("lru"), r.key("observations"), r.key("checkpoint", redisHash(endpointID)))
	result, err := r.client.eval(redisLookupScript, keys, strconv.FormatUint(probe.Global, 10), strconv.FormatUint(probe.FleetGeneration, 10), strconv.FormatUint(probe.EndpointGeneration, 10), strconv.FormatInt(now.UnixMilli(), 10), strconv.FormatInt(probe.CheckpointAt/1e6, 10), strconv.FormatInt(probe.ValidUntil/1e6, 10), strconv.FormatInt(redisCheckpointClaimTTL.Milliseconds(), 10))
	if err != nil {
		return syncResponse{}, false, nil, err
	}
	items, ok := result.([]any)
	if !ok || len(items) == 0 {
		return syncResponse{}, false, nil, nil
	}
	code, _ := strconv.Atoi(asString(items[0]))
	if code == 0 || len(items) < 2 {
		return syncResponse{}, false, nil, nil
	}
	var entry redisEntry
	if err := json.Unmarshal([]byte(asString(items[1])), &entry); err != nil {
		return syncResponse{}, false, nil, nil
	}
	if entry.Fingerprint != fingerprint || entry.ReleaseRef != request.LastReleaseRef || entry.Digest != request.LastDigest || !equalDocumentHashes(entry.Hashes, request.documentHashes.Documents) {
		return syncResponse{}, false, nil, nil
	}
	if code == 2 {
		observations, _ := strconv.ParseUint(asString(items[2]), 10, 64)
		return syncResponse{}, false, &syncCheckpoint{windowStart: time.Unix(0, entry.WindowStart), windowEnd: now, observations: observations, releaseRef: entry.ReleaseRef, digest: entry.Digest, fleet: entry.Fleet, sizeBytes: entry.SizeBytes}, nil
	}
	if now.UnixNano() >= entry.ValidUntil {
		return syncResponse{}, false, nil, nil
	}
	return cloneSyncResponse(entry.Response), true, nil, nil
}

const redisFillScript = `
for i=2,7,2 do if redis.call('EXISTS',KEYS[i+1])==1 then return 0 end end
local gens={ARGV[1],ARGV[2],ARGV[3]}; for i=2,6,2 do local g=redis.call('GET',KEYS[i]); if (g or '0')~=gens[i/2] then return 0 end end
local size=tonumber(ARGV[5]); if size>tonumber(ARGV[8]) then return 0 end
redis.call('SET',KEYS[1],ARGV[4],'PX',ARGV[6]); redis.call('ZADD',KEYS[8],ARGV[7],KEYS[1]); redis.call('HSET',KEYS[9],KEYS[1],size)
local function bytes() local total=0; for _,v in ipairs(redis.call('HVALS',KEYS[9])) do total=total+tonumber(v) end; return total end
while redis.call('ZCARD',KEYS[8])>tonumber(ARGV[9]) or bytes()>tonumber(ARGV[8]) do local v=redis.call('ZRANGE',KEYS[8],0,0)[1]; if not v then break end; redis.call('ZREM',KEYS[8],v); redis.call('HDEL',KEYS[9],v); redis.call('DEL',v) end
return redis.call('EXISTS',KEYS[1])
`

func (r *redisSyncBackend) put(endpointID, fleet, fingerprint string, request syncRequest, response syncResponse, now time.Time, snapshot authoritySnapshot) error {
	checkpointAt := now.Add(r.checkpointInterval)
	validUntil := now.Add(r.ttl)
	if checkpointAt.Before(validUntil) {
		validUntil = checkpointAt
	}
	entry := redisEntry{Fingerprint: fingerprint, Fleet: fleet, Hashes: cloneHashes(response.AcceptedDocumentHashes.Documents), ReleaseRef: response.ReleaseRef, Digest: response.Digest, Response: cloneSyncResponse(response), ValidUntil: validUntil.UnixNano(), CheckpointAt: checkpointAt.UnixNano(), WindowStart: now.UnixNano(), Global: snapshot.global, FleetGeneration: snapshot.fleet, EndpointGeneration: snapshot.endpoint}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	entry.SizeBytes = len(encoded)
	encoded, _ = json.Marshal(entry)
	keys := append([]string{r.endpointKey(endpointID)}, r.scopeKeys(endpointID, fleet)...)
	keys = append(keys, r.key("lru"), r.key("sizes"))
	_, err = r.client.eval(redisFillScript, keys, strconv.FormatUint(snapshot.global, 10), strconv.FormatUint(snapshot.fleet, 10), strconv.FormatUint(snapshot.endpoint, 10), string(encoded), strconv.Itoa(entry.SizeBytes), strconv.FormatInt(r.ttl.Milliseconds(), 10), strconv.FormatInt(now.UnixMilli(), 10), strconv.Itoa(r.maxBytes), strconv.Itoa(r.maxEntries))
	return err
}

func (r *redisSyncBackend) pending(endpointID string) (syncCheckpoint, bool) {
	v, err := r.client.command("GET", r.key("checkpoint", redisHash(endpointID)))
	if err != nil || v == nil {
		return syncCheckpoint{}, false
	}
	var e redisEntry
	if json.Unmarshal([]byte(asString(v)), &e) != nil {
		return syncCheckpoint{}, false
	}
	countValue, _ := r.client.command("HGET", r.key("observations"), r.endpointKey(endpointID))
	count, _ := strconv.ParseUint(asString(countValue), 10, 64)
	return syncCheckpoint{windowStart: time.Unix(0, e.WindowStart), windowEnd: time.Now().UTC(), observations: count, releaseRef: e.ReleaseRef, digest: e.Digest, fleet: e.Fleet, sizeBytes: e.SizeBytes}, true
}
func (r *redisSyncBackend) completeCheckpoint(endpointID string) {
	_, _ = r.client.command("DEL", r.key("checkpoint", redisHash(endpointID)), r.endpointKey(endpointID))
	_, _ = r.client.command("HDEL", r.key("observations"), r.endpointKey(endpointID))
}

func (r *redisSyncBackend) mutation(scope cacheScope, key string) (func(), error) {
	name := "global"
	if scope == cacheScopeFleet {
		name = "fleet"
		key = redisHash(key)
	}
	if scope == cacheScopeEndpoint {
		name = "endpoint"
		key = redisHash(key)
	}
	gen := r.key("generation", name)
	unstable := r.key("unstable", name)
	if name != "global" {
		gen += ":" + key
		unstable += ":" + key
	}
	if _, err := r.client.eval(`redis.call('INCR',KEYS[1]); redis.call('INCR',KEYS[2]); return 1`, []string{gen, unstable}); err != nil {
		return nil, err
	}
	return func() {
		_, _ = r.client.eval(`local n=redis.call('DECR',KEYS[1]); if n<=0 then redis.call('DEL',KEYS[1]) end; return n`, []string{unstable})
	}, nil
}

type respClient struct {
	address, password string
	username          string
	db                int
	tlsConfig         *tls.Config
	pool              chan *respConn
	permits           chan struct{}
}

type respConn struct {
	net.Conn
	rw *bufio.ReadWriter
}

func newRESPClient(raw string) (*respClient, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid redis URL")
	}
	pass, _ := u.User.Password()
	if pass == "" {
		return nil, errors.New("redis URL must contain credentials")
	}
	db := 0
	if strings.TrimPrefix(u.Path, "/") != "" {
		db, err = strconv.Atoi(strings.TrimPrefix(u.Path, "/"))
		if err != nil {
			return nil, errors.New("invalid redis database")
		}
	}
	c := &respClient{address: u.Host, username: u.User.Username(), password: pass, db: db, pool: make(chan *respConn, 64), permits: make(chan struct{}, 64)}
	if u.Scheme == "rediss" {
		c.tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	}
	return c, nil
}
func (c *respClient) command(args ...string) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), redisOperationTimeout)
	defer cancel()
	conn, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	good := false
	defer func() { c.release(conn, good) }()
	_ = conn.SetDeadline(time.Now().Add(redisOperationTimeout))
	if _, err = writeRESP(conn.rw, args); err != nil {
		return nil, err
	}
	if err = conn.rw.Flush(); err != nil {
		return nil, err
	}
	result, err := readRESP(conn.rw.Reader)
	if err != nil {
		return nil, err
	}
	good = true
	return result, nil
}

func (c *respClient) acquire(ctx context.Context) (*respConn, error) {
	select {
	case conn := <-c.pool:
		return conn, nil
	default:
	}
	select {
	case c.permits <- struct{}{}:
		conn, err := c.open(ctx)
		if err != nil {
			<-c.permits
			return nil, err
		}
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *respClient) release(conn *respConn, good bool) {
	if !good {
		_ = conn.Close()
		<-c.permits
		return
	}
	select {
	case c.pool <- conn:
	default:
		_ = conn.Close()
		<-c.permits
	}
}

func (c *respClient) open(ctx context.Context) (*respConn, error) {
	d := net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return nil, err
	}
	var conn net.Conn = raw
	if c.tlsConfig != nil {
		tlsConn := tls.Client(raw, c.tlsConfig.Clone())
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		conn = tlsConn
	}
	_ = conn.SetDeadline(time.Now().Add(redisOperationTimeout))
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	auth := []string{"AUTH", c.password}
	if c.username != "" {
		auth = []string{"AUTH", c.username, c.password}
	}
	if _, err = writeRESP(rw, auth); err != nil {
		conn.Close()
		return nil, err
	}
	if err = rw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err = readRESP(rw.Reader); err != nil {
		conn.Close()
		return nil, err
	}
	if c.db != 0 {
		writeRESP(rw, []string{"SELECT", strconv.Itoa(c.db)})
		rw.Flush()
		if _, err = readRESP(rw.Reader); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return &respConn{Conn: conn, rw: rw}, nil
}
func (c *respClient) eval(script string, keys []string, args ...string) (any, error) {
	cmd := []string{"EVAL", script, strconv.Itoa(len(keys))}
	cmd = append(cmd, keys...)
	cmd = append(cmd, args...)
	return c.command(cmd...)
}
func writeRESP(w io.Writer, args []string) (int, error) {
	n, err := fmt.Fprintf(w, "*%d\r\n", len(args))
	if err != nil {
		return n, err
	}
	for _, a := range args {
		m, e := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(a), a)
		n += m
		if e != nil {
			return n, e
		}
	}
	return n, nil
}
func readRESP(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, errors.New("short redis response")
	}
	body := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch body[0] {
	case '+':
		return body[1:], nil
	case '-':
		return nil, errors.New("redis: " + body[1:])
	case ':':
		return body[1:], nil
	case '$':
		n, _ := strconv.Atoi(body[1:])
		if n < 0 {
			return nil, nil
		}
		b := make([]byte, n+2)
		if _, err = io.ReadFull(r, b); err != nil {
			return nil, err
		}
		return string(b[:n]), nil
	case '*':
		n, _ := strconv.Atoi(body[1:])
		out := make([]any, n)
		for i := range out {
			out[i], err = readRESP(r)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return nil, errors.New("unsupported redis response")
}
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
