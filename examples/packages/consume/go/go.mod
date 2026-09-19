module example.com/consume-go

go 1.22

// Depend on a module from the gitdash Go module proxy.
// Set GOPROXY=http://<user>:<PAT>@<host>/api/packages/go/<owner>,direct
require example.com/<owner>/hello v1.0.0
