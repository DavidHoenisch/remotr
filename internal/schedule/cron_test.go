package schedule

import "testing"

func TestValidateCronExpression_acceptsCommonExprs(t *testing.T) {
	for _, expr := range []string{
		"0 0 * * 0",
		"30 2 * * *",
		"*/15 * * * *",
		"0 0 1 * *",
		"0 9-17 * * 1-5",
	} {
		if err := ValidateCronExpression(expr); err != nil {
			t.Fatalf("expr %q: %v", expr, err)
		}
	}
}

func TestValidateCronExpression_rejectsInvalid(t *testing.T) {
	for _, expr := range []string{
		"",
		"* * *",
		"60 0 * * 0",
		"0 24 * * 0",
		"0 0 32 * *",
	} {
		if err := ValidateCronExpression(expr); err == nil {
			t.Fatalf("expr %q should fail", expr)
		}
	}
}
