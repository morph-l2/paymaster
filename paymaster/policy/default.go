package policy

// DefaultPolicy implements a simple policy
type DefaultPolicy struct {
	name string
}

func (p *DefaultPolicy) Name() string {
	return p.name
}

func (p *DefaultPolicy) IsSponsorable(req SponsorableRequest) bool {
	return true
}
