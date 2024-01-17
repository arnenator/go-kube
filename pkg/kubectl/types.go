package kubectl

type uxeported interface {
	unexported()
}
type DryRunType uint8

const (
	DryRunNone DryRunType = iota
	DryRunClient
	DryRunServer
)

func (d DryRunType) String() string {
	return [...]string{"none", "client", "server"}[d]
}

// We implement the unexported interface to make sure that the DryRunType
// cannot be extended/changed outside the package
func (d DryRunType) unexported() {}
