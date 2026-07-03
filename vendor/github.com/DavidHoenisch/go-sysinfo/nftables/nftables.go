package nftables

import (
	"encoding/json"
)

type TableSummary struct {
	Family string
	Name   string
}

type ChainSummary struct {
	Table     string
	Family    string
	Name      string
	Type      string
	Hook      string
	RuleCount int
}

type RulesetSummary struct {
	Tables []TableSummary
	Chains []ChainSummary
}

type nftablesJSON struct {
	Nftables []json.RawMessage `json:"nftables"`
}

func Available(r Reader) bool {
	_, err := r.cmd().Run("nft", "--version")
	return err == nil
}

func (r Reader) Available() bool {
	return Available(r)
}

func GetRulesetSummary(r Reader) (*RulesetSummary, error) {
	if !Available(r) {
		return nil, nil
	}
	out, err := r.cmd().Run("nft", "-j", "list", "ruleset")
	if err != nil {
		return handlePtrError[RulesetSummary](r, err)
	}
	return parseRulesetSummary(string(out)), nil
}

func (r Reader) GetRulesetSummary() (*RulesetSummary, error) {
	return GetRulesetSummary(r)
}

func handlePtrError[T any](r Reader, err error) (*T, error) {
	if r.Privileged {
		return nil, err
	}
	return nil, nil
}

func parseRulesetSummary(content string) *RulesetSummary {
	if content == "" {
		return &RulesetSummary{}
	}
	var root nftablesJSON
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return &RulesetSummary{}
	}

	summary := &RulesetSummary{}

	// First pass: collect tables
	tables := make(map[string]TableSummary)
	for _, raw := range root.Nftables {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			continue
		}
		if tblRaw, ok := wrapper["table"]; ok {
			var tbl struct {
				Family string `json:"family"`
				Name   string `json:"name"`
			}
			if err := json.Unmarshal(tblRaw, &tbl); err == nil {
				key := tbl.Family + "/" + tbl.Name
				tables[key] = TableSummary{Family: tbl.Family, Name: tbl.Name}
			}
		}
	}

	// Second pass: collect chains and count rules per chain
	chainRuleCounts := make(map[string]int)
	chains := make(map[string]ChainSummary)
	for _, raw := range root.Nftables {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			continue
		}
		if chainRaw, ok := wrapper["chain"]; ok {
			var chain struct {
				Family string `json:"family"`
				Table  string `json:"table"`
				Name   string `json:"name"`
				Type   string `json:"type"`
				Hook   string `json:"hook"`
			}
			if err := json.Unmarshal(chainRaw, &chain); err == nil {
				key := chain.Family + "/" + chain.Table + "/" + chain.Name
				chains[key] = ChainSummary{
					Table:  chain.Table,
					Family: chain.Family,
					Name:   chain.Name,
					Type:   chain.Type,
					Hook:   chain.Hook,
				}
			}
		}
		if ruleRaw, ok := wrapper["rule"]; ok {
			var rule struct {
				Family string `json:"family"`
				Table  string `json:"table"`
				Chain  string `json:"chain"`
			}
			if err := json.Unmarshal(ruleRaw, &rule); err == nil {
				key := rule.Family + "/" + rule.Table + "/" + rule.Chain
				chainRuleCounts[key]++
			}
		}
	}

	for _, t := range tables {
		summary.Tables = append(summary.Tables, t)
	}
	for key, c := range chains {
		c.RuleCount = chainRuleCounts[key]
		summary.Chains = append(summary.Chains, c)
	}
	return summary
}
