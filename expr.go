package plots

import (
	"math"
	"strconv"
	"strings"
)

// evalSimpleExpr evaluates simple arithmetic expressions used in worlds.yml
// economy prices (e.g. "100", "100 * plots", "100 + 50 * plots", "100 * 2^plots").
func evalSimpleExpr(expr string, plots float64) (float64, bool) {
	p := &exprParser{s: strings.TrimSpace(expr), plots: plots}
	v, ok := p.parseAddSub()
	p.skipSpace()
	if !ok || p.pos != len(p.s) {
		return 0, false
	}
	return v, true
}

type exprParser struct {
	s     string
	pos   int
	plots float64
}

func (p *exprParser) skipSpace() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) parseAddSub() (float64, bool) {
	left, ok := p.parseMulDiv()
	if !ok {
		return 0, false
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return left, true
		}
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			return left, true
		}
		p.pos++
		right, ok := p.parseMulDiv()
		if !ok {
			return 0, false
		}
		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}
}

func (p *exprParser) parseMulDiv() (float64, bool) {
	left, ok := p.parsePow()
	if !ok {
		return 0, false
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return left, true
		}
		op := p.s[p.pos]
		if op != '*' && op != '/' {
			return left, true
		}
		p.pos++
		right, ok := p.parsePow()
		if !ok {
			return 0, false
		}
		if op == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, false
			}
			left /= right
		}
	}
}

func (p *exprParser) parsePow() (float64, bool) {
	base, ok := p.parseUnary()
	if !ok {
		return 0, false
	}
	p.skipSpace()
	if p.pos < len(p.s) && p.s[p.pos] == '^' {
		p.pos++
		exp, ok := p.parsePow()
		if !ok {
			return 0, false
		}
		return math.Pow(base, exp), true
	}
	return base, true
}

func (p *exprParser) parseUnary() (float64, bool) {
	p.skipSpace()
	if p.pos < len(p.s) && p.s[p.pos] == '-' {
		p.pos++
		v, ok := p.parseUnary()
		return -v, ok
	}
	if p.pos < len(p.s) && p.s[p.pos] == '+' {
		p.pos++
		return p.parseUnary()
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (float64, bool) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return 0, false
	}
	if p.s[p.pos] == '(' {
		p.pos++
		v, ok := p.parseAddSub()
		p.skipSpace()
		if !ok || p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, false
		}
		p.pos++
		return v, true
	}
	if strings.HasPrefix(strings.ToLower(p.s[p.pos:]), "plots") {
		p.pos += 5
		return p.plots, true
	}
	start := p.pos
	for p.pos < len(p.s) && ((p.s[p.pos] >= '0' && p.s[p.pos] <= '9') || p.s[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return 0, false
	}
	v, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
