go-winres make

set CGO_ENABLED=1
set GOOS=windows

set GOARCH=amd64
go build -o ImageTrim64.exe -ldflags="-s -w -H windowsgui"

set GOARCH=386
go build -o ImageTrim32.exe -ldflags="-s -w -H windowsgui"
