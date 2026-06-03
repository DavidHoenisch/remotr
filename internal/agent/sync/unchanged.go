package sync

// Unchanged reports whether the server may skip sending artifact bytes.
// Digest match alone is insufficient when the global release ref advanced:
// the agent must re-check (and optionally re-apply) even if file content is identical.
func Unchanged(lastDigest, serverDigest, lastReleaseRef, serverReleaseRef string) bool {
	if lastDigest != serverDigest {
		return false
	}
	if serverReleaseRef == "" {
		return true
	}
	if lastReleaseRef == "" {
		return false
	}
	return lastReleaseRef == serverReleaseRef
}
