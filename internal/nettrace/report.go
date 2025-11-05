package nettrace

// Report summarises a collected timeline along with evaluated budgets.
type Report struct {
	Timeline     *Timeline
	Budget       Budget
	BudgetReport BudgetReport
}

// NewReport builds a trace report for the provided timeline and budget.
// The timeline is cloned to keep the report immutable.
func NewReport(tl *Timeline, budget Budget) *Report {
	if tl == nil {
		return nil
	}

	cloned := tl.Clone()
	budgetCopy := budget.Clone()

	var evaluation BudgetReport
	if hasBudget(budgetCopy) {
		evaluation = EvaluateBudget(cloned, budgetCopy)
	}

	return &Report{
		Timeline:     cloned,
		Budget:       budgetCopy,
		BudgetReport: evaluation,
	}
}

// hasBudget reports whether any limits are configured on the budget.
func hasBudget(b Budget) bool {
	if b.Total > 0 || b.Tolerance > 0 {
		return true
	}
	return len(b.Phases) > 0
}

// Clone returns a deep copy of the report so callers can safely mutate it.
func (r *Report) Clone() *Report {
	if r == nil {
		return nil
	}

	clone := &Report{
		Budget: r.Budget.Clone(),
	}

	if r.Timeline != nil {
		clone.Timeline = r.Timeline.Clone()
	}

	if len(r.BudgetReport.Breaches) > 0 {
		clone.BudgetReport.Breaches = make([]BudgetBreach, len(r.BudgetReport.Breaches))
		copy(clone.BudgetReport.Breaches, r.BudgetReport.Breaches)
	}

	return clone
}
