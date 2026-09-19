module example.com/consume-go

go 1.22

// Depend on the module published from ../hello (same module path as its go.mod).
// Set GOPROXY=http://<user>:<PAT>@<host>/api/packages/go/<owner>,direct
require example.com/gitdash/hello v1.0.0
