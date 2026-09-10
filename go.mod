module github.com/yeoblyv/diskette

go 1.26.0

require (
	github.com/pkg/sftp v1.13.11
	github.com/yeoblyv/graphite v0.0.0
	golang.org/x/crypto v0.57.0
	golang.org/x/sys v0.48.0
)

require (
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/kr/fs v0.1.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/yeoblyv/graphite => ../graphite
