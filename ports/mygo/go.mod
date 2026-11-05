module github.com/ferrumlux/noonmark/ports/mygo

go 1.27.3

require github.com/egoist/mygo v0.7.5-0.20261105040656-977ea57e0baf

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

require (
 github.com/ferrumlux/noonmark/ports/mygo/domain v0.0.0
 github.com/ferrumlux/noonmark/ports/mygo/domain-extensions v0.0.0
 github.com/ferrumlux/noonmark/ports/mygo/domain-codec v0.0.0
)

replace github.com/ferrumlux/noonmark/ports/mygo/domain => ./domain-core
replace github.com/ferrumlux/noonmark/ports/mygo/domain-extensions => ./domain-extensions
replace github.com/ferrumlux/noonmark/ports/mygo/domain-codec => ./domain-codec
