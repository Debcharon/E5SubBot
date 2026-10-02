package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string { return fmt.Sprintf("E5SubBot %s (commit %s, built %s)", Version, Commit, Date) }
