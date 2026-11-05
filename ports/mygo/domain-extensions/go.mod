module github.com/ferrumlux/noonmark/ports/mygo/domain-extensions

go 1.27.3

require github.com/ferrumlux/noonmark/ports/mygo/domain v0.0.0

require golang.org/x/text v0.43.0 // indirect

replace github.com/ferrumlux/noonmark/ports/mygo/domain => ../domain-core
