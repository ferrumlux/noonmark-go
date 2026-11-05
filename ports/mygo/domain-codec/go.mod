module github.com/ferrumlux/noonmark/ports/mygo/domain-codec

go 1.23.0

require (
 github.com/ferrumlux/noonmark/ports/mygo/domain v0.0.0
 github.com/ferrumlux/noonmark/ports/mygo/domain-extensions v0.0.0
)
replace github.com/ferrumlux/noonmark/ports/mygo/domain => ../domain-core
replace github.com/ferrumlux/noonmark/ports/mygo/domain-extensions => ../domain-extensions
