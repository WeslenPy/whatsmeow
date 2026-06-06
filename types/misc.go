package types

type ReachoutTimeoutLock struct {
	IsActive            bool `json:"is_active"`
	TimeEnforcementEnds any  `json:"time_enforcement_ends"`
}
