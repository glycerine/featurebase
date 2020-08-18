package pql

//go:generate peg -inline pql.peg

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

const endSymbol rune = 1114112

/* The rule types inferred from the grammar are below. */
type pegRule uint8

const (
	ruleUnknown pegRule = iota
	ruleCalls
	ruleCall
	ruleallargs
	rulefargs
	rulefarg
	ruledargs
	ruledarg
	ruleCOND
	ruleconditional
	rulecondint
	rulecondLT
	rulecondfield
	ruledvalue
	rulefvalue
	ruledlist
	ruleflist
	ruleditem
	rulefitem
	ruleitema
	ruleitemb
	rulefloat
	ruledecimal
	ruledoublequotedstring
	rulesinglequotedstring
	rulefieldExpr
	rulefield
	rulereserved
	ruleposfield
	ruleuint
	rulecol
	rulerow
	ruleopen
	ruleclose
	rulesp
	rulecomma
	rulelbrack
	rulerbrack
	ruleIDENT
	ruletimestampbasicfmt
	ruletimestampfmt
	ruletimestamp
	ruleAction0
	ruleAction1
	ruleAction2
	ruleAction3
	ruleAction4
	ruleAction5
	ruleAction6
	ruleAction7
	ruleAction8
	ruleAction9
	ruleAction10
	ruleAction11
	ruleAction12
	ruleAction13
	ruleAction14
	ruleAction15
	ruleAction16
	ruleAction17
	ruleAction18
	ruleAction19
	ruleAction20
	ruleAction21
	rulePegText
	ruleAction22
	ruleAction23
	ruleAction24
	ruleAction25
	ruleAction26
	ruleAction27
	ruleAction28
	ruleAction29
	ruleAction30
	ruleAction31
	ruleAction32
	ruleAction33
	ruleAction34
	ruleAction35
	ruleAction36
	ruleAction37
	ruleAction38
	ruleAction39
	ruleAction40
	ruleAction41
	ruleAction42
	ruleAction43
	ruleAction44
	ruleAction45
	ruleAction46
	ruleAction47
	ruleAction48
	ruleAction49
	ruleAction50
	ruleAction51
	ruleAction52
	ruleAction53
	ruleAction54
	ruleAction55
	ruleAction56
	ruleAction57
	ruleAction58
	ruleAction59
	ruleAction60
	ruleAction61
)

var rul3s = [...]string{
	"Unknown",
	"Calls",
	"Call",
	"allargs",
	"fargs",
	"farg",
	"dargs",
	"darg",
	"COND",
	"conditional",
	"condint",
	"condLT",
	"condfield",
	"dvalue",
	"fvalue",
	"dlist",
	"flist",
	"ditem",
	"fitem",
	"itema",
	"itemb",
	"float",
	"decimal",
	"doublequotedstring",
	"singlequotedstring",
	"fieldExpr",
	"field",
	"reserved",
	"posfield",
	"uint",
	"col",
	"row",
	"open",
	"close",
	"sp",
	"comma",
	"lbrack",
	"rbrack",
	"IDENT",
	"timestampbasicfmt",
	"timestampfmt",
	"timestamp",
	"Action0",
	"Action1",
	"Action2",
	"Action3",
	"Action4",
	"Action5",
	"Action6",
	"Action7",
	"Action8",
	"Action9",
	"Action10",
	"Action11",
	"Action12",
	"Action13",
	"Action14",
	"Action15",
	"Action16",
	"Action17",
	"Action18",
	"Action19",
	"Action20",
	"Action21",
	"PegText",
	"Action22",
	"Action23",
	"Action24",
	"Action25",
	"Action26",
	"Action27",
	"Action28",
	"Action29",
	"Action30",
	"Action31",
	"Action32",
	"Action33",
	"Action34",
	"Action35",
	"Action36",
	"Action37",
	"Action38",
	"Action39",
	"Action40",
	"Action41",
	"Action42",
	"Action43",
	"Action44",
	"Action45",
	"Action46",
	"Action47",
	"Action48",
	"Action49",
	"Action50",
	"Action51",
	"Action52",
	"Action53",
	"Action54",
	"Action55",
	"Action56",
	"Action57",
	"Action58",
	"Action59",
	"Action60",
	"Action61",
}

type token32 struct {
	pegRule
	begin, end uint32
}

func (t *token32) String() string {
	return fmt.Sprintf("\x1B[34m%v\x1B[m %v %v", rul3s[t.pegRule], t.begin, t.end)
}

type node32 struct {
	token32
	up, next *node32
}

func (node *node32) print(pretty bool, buffer string) {
	var print func(node *node32, depth int)
	print = func(node *node32, depth int) {
		for node != nil {
			for c := 0; c < depth; c++ {
				fmt.Printf(" ")
			}
			rule := rul3s[node.pegRule]
			quote := strconv.Quote(string(([]rune(buffer)[node.begin:node.end])))
			if !pretty {
				fmt.Printf("%v %v\n", rule, quote)
			} else {
				fmt.Printf("\x1B[34m%v\x1B[m %v\n", rule, quote)
			}
			if node.up != nil {
				print(node.up, depth+1)
			}
			node = node.next
		}
	}
	print(node, 0)
}

func (node *node32) Print(buffer string) {
	node.print(false, buffer)
}

func (node *node32) PrettyPrint(buffer string) {
	node.print(true, buffer)
}

type tokens32 struct {
	tree []token32
}

func (t *tokens32) Trim(length uint32) {
	t.tree = t.tree[:length]
}

func (t *tokens32) Print() {
	for _, token := range t.tree {
		fmt.Println(token.String())
	}
}

func (t *tokens32) AST() *node32 {
	type element struct {
		node *node32
		down *element
	}
	tokens := t.Tokens()
	var stack *element
	for _, token := range tokens {
		if token.begin == token.end {
			continue
		}
		node := &node32{token32: token}
		for stack != nil && stack.node.begin >= token.begin && stack.node.end <= token.end {
			stack.node.next = node.up
			node.up = stack.node
			stack = stack.down
		}
		stack = &element{node: node, down: stack}
	}
	if stack != nil {
		return stack.node
	}
	return nil
}

func (t *tokens32) PrintSyntaxTree(buffer string) {
	t.AST().Print(buffer)
}

func (t *tokens32) PrettyPrintSyntaxTree(buffer string) {
	t.AST().PrettyPrint(buffer)
}

func (t *tokens32) Add(rule pegRule, begin, end, index uint32) {
	if tree := t.tree; int(index) >= len(tree) {
		expanded := make([]token32, 2*len(tree))
		copy(expanded, tree)
		t.tree = expanded
	}
	t.tree[index] = token32{
		pegRule: rule,
		begin:   begin,
		end:     end,
	}
}

func (t *tokens32) Tokens() []token32 {
	return t.tree
}

type PQL struct {
	Query

	Buffer string
	buffer []rune
	rules  [105]func() bool
	parse  func(rule ...int) error
	reset  func()
	Pretty bool
	tokens32
}

func (p *PQL) Parse(rule ...int) error {
	return p.parse(rule...)
}

func (p *PQL) Reset() {
	p.reset()
}

type textPosition struct {
	line, symbol int
}

type textPositionMap map[int]textPosition

func translatePositions(buffer []rune, positions []int) textPositionMap {
	length, translations, j, line, symbol := len(positions), make(textPositionMap, len(positions)), 0, 1, 0
	sort.Ints(positions)

search:
	for i, c := range buffer {
		if c == '\n' {
			line, symbol = line+1, 0
		} else {
			symbol++
		}
		if i == positions[j] {
			translations[positions[j]] = textPosition{line, symbol}
			for j++; j < length; j++ {
				if i != positions[j] {
					continue search
				}
			}
			break search
		}
	}

	return translations
}

type parseError struct {
	p   *PQL
	max token32
}

func (e *parseError) Error() string {
	tokens, error := []token32{e.max}, "\n"
	positions, p := make([]int, 2*len(tokens)), 0
	for _, token := range tokens {
		positions[p], p = int(token.begin), p+1
		positions[p], p = int(token.end), p+1
	}
	translations := translatePositions(e.p.buffer, positions)
	format := "parse error near %v (line %v symbol %v - line %v symbol %v):\n%v\n"
	if e.p.Pretty {
		format = "parse error near \x1B[34m%v\x1B[m (line %v symbol %v - line %v symbol %v):\n%v\n"
	}
	for _, token := range tokens {
		begin, end := int(token.begin), int(token.end)
		error += fmt.Sprintf(format,
			rul3s[token.pegRule],
			translations[begin].line, translations[begin].symbol,
			translations[end].line, translations[end].symbol,
			strconv.Quote(string(e.p.buffer[begin:end])))
	}

	return error
}

func (p *PQL) PrintSyntaxTree() {
	if p.Pretty {
		p.tokens32.PrettyPrintSyntaxTree(p.Buffer)
	} else {
		p.tokens32.PrintSyntaxTree(p.Buffer)
	}
}

func (p *PQL) Execute() {
	buffer, _buffer, text, begin, end := p.Buffer, p.buffer, "", 0, 0
	for _, token := range p.Tokens() {
		switch token.pegRule {

		case rulePegText:
			begin, end = int(token.begin), int(token.end)
			text = string(_buffer[begin:end])

		case ruleAction0:
			p.startCall("Set")
		case ruleAction1:
			p.endCall()
		case ruleAction2:
			p.startCall("SetRowAttrs")
		case ruleAction3:
			p.endCall()
		case ruleAction4:
			p.startCall("SetColumnAttrs")
		case ruleAction5:
			p.endCall()
		case ruleAction6:
			p.startCall("Clear")
		case ruleAction7:
			p.endCall()
		case ruleAction8:
			p.startCall("ClearRow")
		case ruleAction9:
			p.endCall()
		case ruleAction10:
			p.startCall("Store")
		case ruleAction11:
			p.endCall()
		case ruleAction12:
			p.startCall("TopN")
		case ruleAction13:
			p.endCall()
		case ruleAction14:
			p.startCall("Rows")
		case ruleAction15:
			p.endCall()
		case ruleAction16:
			p.startCall("Range")
		case ruleAction17:
			p.addField("from")
		case ruleAction18:
			p.addVal(buffer[begin:end])
		case ruleAction19:
			p.addField("to")
		case ruleAction20:
			p.addVal(buffer[begin:end])
		case ruleAction21:
			p.endCall()
		case ruleAction22:
			p.startCall(buffer[begin:end])
		case ruleAction23:
			p.endCall()
		case ruleAction24:
			p.addBTWN()
		case ruleAction25:
			p.addLTE()
		case ruleAction26:
			p.addGTE()
		case ruleAction27:
			p.addEQ()
		case ruleAction28:
			p.addNEQ()
		case ruleAction29:
			p.addLT()
		case ruleAction30:
			p.addGT()
		case ruleAction31:
			p.startConditional()
		case ruleAction32:
			p.endConditional()
		case ruleAction33:
			p.condAdd(buffer[begin:end])
		case ruleAction34:
			p.condAdd(buffer[begin:end])
		case ruleAction35:
			p.condAdd(buffer[begin:end])
		case ruleAction36:
			p.startList()
		case ruleAction37:
			p.endList()
		case ruleAction38:
			p.startList()
		case ruleAction39:
			p.endList()
		case ruleAction40:
			p.addVal(nil)
		case ruleAction41:
			p.addVal(true)
		case ruleAction42:
			p.addVal(false)
		case ruleAction43:
			p.addVal(buffer[begin:end])
		case ruleAction44:
			p.startCall(string(_buffer[begin:end]))
		case ruleAction45:
			p.addVal(p.endCall())
		case ruleAction46:
			p.addVal(string(_buffer[begin:end]))
		case ruleAction47:
			p.addVal(string(_buffer[begin:end]))
		case ruleAction48:
			p.addVal(string(_buffer[begin:end]))
		case ruleAction49:
			p.addNumVal(buffer[begin:end], true)
		case ruleAction50:
			p.addNumVal(buffer[begin:end], true)
		case ruleAction51:
			p.addNumVal(buffer[begin:end], false)
		case ruleAction52:
			p.addNumVal(buffer[begin:end], false)
		case ruleAction53:
			p.addField(buffer[begin:end])
		case ruleAction54:
			p.addPosStr("_field", buffer[begin:end])
		case ruleAction55:
			p.addPosNum("_col", buffer[begin:end])
		case ruleAction56:
			p.addPosStr("_col", buffer[begin:end])
		case ruleAction57:
			p.addPosStr("_col", buffer[begin:end])
		case ruleAction58:
			p.addPosNum("_row", buffer[begin:end])
		case ruleAction59:
			p.addPosStr("_row", buffer[begin:end])
		case ruleAction60:
			p.addPosStr("_row", buffer[begin:end])
		case ruleAction61:
			p.addPosStr("_timestamp", buffer[begin:end])

		}
	}
	_, _, _, _, _ = buffer, _buffer, text, begin, end
}

func (p *PQL) Init() {
	var (
		max                  token32
		position, tokenIndex uint32
		buffer               []rune
	)
	p.reset = func() {
		max = token32{}
		position, tokenIndex = 0, 0

		p.buffer = []rune(p.Buffer)
		if len(p.buffer) == 0 || p.buffer[len(p.buffer)-1] != endSymbol {
			p.buffer = append(p.buffer, endSymbol)
		}
		buffer = p.buffer
	}
	p.reset()

	_rules := p.rules
	tree := tokens32{tree: make([]token32, math.MaxInt16)}
	p.parse = func(rule ...int) error {
		r := 1
		if len(rule) > 0 {
			r = rule[0]
		}
		matches := p.rules[r]()
		p.tokens32 = tree
		if matches {
			p.Trim(tokenIndex)
			return nil
		}
		return &parseError{p, max}
	}

	add := func(rule pegRule, begin uint32) {
		tree.Add(rule, begin, position, tokenIndex)
		tokenIndex++
		if begin != position && position > max.end {
			max = token32{rule, begin, position}
		}
	}

	matchDot := func() bool {
		if buffer[position] != endSymbol {
			position++
			return true
		}
		return false
	}

	/*matchChar := func(c byte) bool {
		if buffer[position] == c {
			position++
			return true
		}
		return false
	}*/

	/*matchRange := func(lower byte, upper byte) bool {
		if c := buffer[position]; c >= lower && c <= upper {
			position++
			return true
		}
		return false
	}*/

	_rules = [...]func() bool{
		nil,
		/* 0 Calls <- <(sp (Call sp)* !.)> */
		func() bool {
			position0, tokenIndex0 := position, tokenIndex
			{
				position1 := position
				if !_rules[rulesp]() {
					goto l0
				}
			l2:
				{
					position3, tokenIndex3 := position, tokenIndex
					if !_rules[ruleCall]() {
						goto l3
					}
					if !_rules[rulesp]() {
						goto l3
					}
					goto l2
				l3:
					position, tokenIndex = position3, tokenIndex3
				}
				{
					position4, tokenIndex4 := position, tokenIndex
					if !matchDot() {
						goto l4
					}
					goto l0
				l4:
					position, tokenIndex = position4, tokenIndex4
				}
				add(ruleCalls, position1)
			}
			return true
		l0:
			position, tokenIndex = position0, tokenIndex0
			return false
		},
		/* 1 Call <- <(('S' 'e' 't' Action0 open col comma dargs (comma timestamp)? close Action1) / ('S' 'e' 't' 'R' 'o' 'w' 'A' 't' 't' 'r' 's' Action2 open posfield comma row comma fargs close Action3) / ('S' 'e' 't' 'C' 'o' 'l' 'u' 'm' 'n' 'A' 't' 't' 'r' 's' Action4 open col comma fargs close Action5) / ('C' 'l' 'e' 'a' 'r' Action6 open col comma dargs close Action7) / ('C' 'l' 'e' 'a' 'r' 'R' 'o' 'w' Action8 open darg close Action9) / ('S' 't' 'o' 'r' 'e' Action10 open Call comma darg close Action11) / ('T' 'o' 'p' 'N' Action12 open posfield (comma allargs)? close Action13) / ('R' 'o' 'w' 's' Action14 open posfield (comma allargs)? close Action15) / ('R' 'a' 'n' 'g' 'e' Action16 open field sp '=' sp fvalue comma ('f' 'r' 'o' 'm' '=')? Action17 timestampfmt Action18 comma ('t' 'o' '=')? sp Action19 timestampfmt Action20 close Action21) / (<IDENT> Action22 open allargs comma? close Action23))> */
		func() bool {
			position5, tokenIndex5 := position, tokenIndex
			{
				position6 := position
				{
					position7, tokenIndex7 := position, tokenIndex
					if buffer[position] != rune('S') {
						goto l8
					}
					position++
					if buffer[position] != rune('e') {
						goto l8
					}
					position++
					if buffer[position] != rune('t') {
						goto l8
					}
					position++
					{
						add(ruleAction0, position)
					}
					if !_rules[ruleopen]() {
						goto l8
					}
					if !_rules[rulecol]() {
						goto l8
					}
					if !_rules[rulecomma]() {
						goto l8
					}
					if !_rules[ruledargs]() {
						goto l8
					}
					{
						position10, tokenIndex10 := position, tokenIndex
						if !_rules[rulecomma]() {
							goto l10
						}
						{
							position12 := position
							{
								position13 := position
								if !_rules[ruletimestampfmt]() {
									goto l10
								}
								add(rulePegText, position13)
							}
							{
								add(ruleAction61, position)
							}
							add(ruletimestamp, position12)
						}
						goto l11
					l10:
						position, tokenIndex = position10, tokenIndex10
					}
				l11:
					if !_rules[ruleclose]() {
						goto l8
					}
					{
						add(ruleAction1, position)
					}
					goto l7
				l8:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('S') {
						goto l16
					}
					position++
					if buffer[position] != rune('e') {
						goto l16
					}
					position++
					if buffer[position] != rune('t') {
						goto l16
					}
					position++
					if buffer[position] != rune('R') {
						goto l16
					}
					position++
					if buffer[position] != rune('o') {
						goto l16
					}
					position++
					if buffer[position] != rune('w') {
						goto l16
					}
					position++
					if buffer[position] != rune('A') {
						goto l16
					}
					position++
					if buffer[position] != rune('t') {
						goto l16
					}
					position++
					if buffer[position] != rune('t') {
						goto l16
					}
					position++
					if buffer[position] != rune('r') {
						goto l16
					}
					position++
					if buffer[position] != rune('s') {
						goto l16
					}
					position++
					{
						add(ruleAction2, position)
					}
					if !_rules[ruleopen]() {
						goto l16
					}
					if !_rules[ruleposfield]() {
						goto l16
					}
					if !_rules[rulecomma]() {
						goto l16
					}
					{
						position18 := position
						{
							position19, tokenIndex19 := position, tokenIndex
							{
								position21 := position
								if !_rules[ruleuint]() {
									goto l20
								}
								add(rulePegText, position21)
							}
							{
								add(ruleAction58, position)
							}
							goto l19
						l20:
							position, tokenIndex = position19, tokenIndex19
							{
								position24 := position
								if buffer[position] != rune('\'') {
									goto l23
								}
								position++
								if !_rules[rulesinglequotedstring]() {
									goto l23
								}
								if buffer[position] != rune('\'') {
									goto l23
								}
								position++
								add(rulePegText, position24)
							}
							{
								add(ruleAction59, position)
							}
							goto l19
						l23:
							position, tokenIndex = position19, tokenIndex19
							{
								position26 := position
								if buffer[position] != rune('"') {
									goto l16
								}
								position++
								if !_rules[ruledoublequotedstring]() {
									goto l16
								}
								if buffer[position] != rune('"') {
									goto l16
								}
								position++
								add(rulePegText, position26)
							}
							{
								add(ruleAction60, position)
							}
						}
					l19:
						add(rulerow, position18)
					}
					if !_rules[rulecomma]() {
						goto l16
					}
					if !_rules[rulefargs]() {
						goto l16
					}
					if !_rules[ruleclose]() {
						goto l16
					}
					{
						add(ruleAction3, position)
					}
					goto l7
				l16:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('S') {
						goto l29
					}
					position++
					if buffer[position] != rune('e') {
						goto l29
					}
					position++
					if buffer[position] != rune('t') {
						goto l29
					}
					position++
					if buffer[position] != rune('C') {
						goto l29
					}
					position++
					if buffer[position] != rune('o') {
						goto l29
					}
					position++
					if buffer[position] != rune('l') {
						goto l29
					}
					position++
					if buffer[position] != rune('u') {
						goto l29
					}
					position++
					if buffer[position] != rune('m') {
						goto l29
					}
					position++
					if buffer[position] != rune('n') {
						goto l29
					}
					position++
					if buffer[position] != rune('A') {
						goto l29
					}
					position++
					if buffer[position] != rune('t') {
						goto l29
					}
					position++
					if buffer[position] != rune('t') {
						goto l29
					}
					position++
					if buffer[position] != rune('r') {
						goto l29
					}
					position++
					if buffer[position] != rune('s') {
						goto l29
					}
					position++
					{
						add(ruleAction4, position)
					}
					if !_rules[ruleopen]() {
						goto l29
					}
					if !_rules[rulecol]() {
						goto l29
					}
					if !_rules[rulecomma]() {
						goto l29
					}
					if !_rules[rulefargs]() {
						goto l29
					}
					if !_rules[ruleclose]() {
						goto l29
					}
					{
						add(ruleAction5, position)
					}
					goto l7
				l29:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('C') {
						goto l32
					}
					position++
					if buffer[position] != rune('l') {
						goto l32
					}
					position++
					if buffer[position] != rune('e') {
						goto l32
					}
					position++
					if buffer[position] != rune('a') {
						goto l32
					}
					position++
					if buffer[position] != rune('r') {
						goto l32
					}
					position++
					{
						add(ruleAction6, position)
					}
					if !_rules[ruleopen]() {
						goto l32
					}
					if !_rules[rulecol]() {
						goto l32
					}
					if !_rules[rulecomma]() {
						goto l32
					}
					if !_rules[ruledargs]() {
						goto l32
					}
					if !_rules[ruleclose]() {
						goto l32
					}
					{
						add(ruleAction7, position)
					}
					goto l7
				l32:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('C') {
						goto l35
					}
					position++
					if buffer[position] != rune('l') {
						goto l35
					}
					position++
					if buffer[position] != rune('e') {
						goto l35
					}
					position++
					if buffer[position] != rune('a') {
						goto l35
					}
					position++
					if buffer[position] != rune('r') {
						goto l35
					}
					position++
					if buffer[position] != rune('R') {
						goto l35
					}
					position++
					if buffer[position] != rune('o') {
						goto l35
					}
					position++
					if buffer[position] != rune('w') {
						goto l35
					}
					position++
					{
						add(ruleAction8, position)
					}
					if !_rules[ruleopen]() {
						goto l35
					}
					if !_rules[ruledarg]() {
						goto l35
					}
					if !_rules[ruleclose]() {
						goto l35
					}
					{
						add(ruleAction9, position)
					}
					goto l7
				l35:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('S') {
						goto l38
					}
					position++
					if buffer[position] != rune('t') {
						goto l38
					}
					position++
					if buffer[position] != rune('o') {
						goto l38
					}
					position++
					if buffer[position] != rune('r') {
						goto l38
					}
					position++
					if buffer[position] != rune('e') {
						goto l38
					}
					position++
					{
						add(ruleAction10, position)
					}
					if !_rules[ruleopen]() {
						goto l38
					}
					if !_rules[ruleCall]() {
						goto l38
					}
					if !_rules[rulecomma]() {
						goto l38
					}
					if !_rules[ruledarg]() {
						goto l38
					}
					if !_rules[ruleclose]() {
						goto l38
					}
					{
						add(ruleAction11, position)
					}
					goto l7
				l38:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('T') {
						goto l41
					}
					position++
					if buffer[position] != rune('o') {
						goto l41
					}
					position++
					if buffer[position] != rune('p') {
						goto l41
					}
					position++
					if buffer[position] != rune('N') {
						goto l41
					}
					position++
					{
						add(ruleAction12, position)
					}
					if !_rules[ruleopen]() {
						goto l41
					}
					if !_rules[ruleposfield]() {
						goto l41
					}
					{
						position43, tokenIndex43 := position, tokenIndex
						if !_rules[rulecomma]() {
							goto l43
						}
						if !_rules[ruleallargs]() {
							goto l43
						}
						goto l44
					l43:
						position, tokenIndex = position43, tokenIndex43
					}
				l44:
					if !_rules[ruleclose]() {
						goto l41
					}
					{
						add(ruleAction13, position)
					}
					goto l7
				l41:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('R') {
						goto l46
					}
					position++
					if buffer[position] != rune('o') {
						goto l46
					}
					position++
					if buffer[position] != rune('w') {
						goto l46
					}
					position++
					if buffer[position] != rune('s') {
						goto l46
					}
					position++
					{
						add(ruleAction14, position)
					}
					if !_rules[ruleopen]() {
						goto l46
					}
					if !_rules[ruleposfield]() {
						goto l46
					}
					{
						position48, tokenIndex48 := position, tokenIndex
						if !_rules[rulecomma]() {
							goto l48
						}
						if !_rules[ruleallargs]() {
							goto l48
						}
						goto l49
					l48:
						position, tokenIndex = position48, tokenIndex48
					}
				l49:
					if !_rules[ruleclose]() {
						goto l46
					}
					{
						add(ruleAction15, position)
					}
					goto l7
				l46:
					position, tokenIndex = position7, tokenIndex7
					if buffer[position] != rune('R') {
						goto l51
					}
					position++
					if buffer[position] != rune('a') {
						goto l51
					}
					position++
					if buffer[position] != rune('n') {
						goto l51
					}
					position++
					if buffer[position] != rune('g') {
						goto l51
					}
					position++
					if buffer[position] != rune('e') {
						goto l51
					}
					position++
					{
						add(ruleAction16, position)
					}
					if !_rules[ruleopen]() {
						goto l51
					}
					if !_rules[rulefield]() {
						goto l51
					}
					if !_rules[rulesp]() {
						goto l51
					}
					if buffer[position] != rune('=') {
						goto l51
					}
					position++
					if !_rules[rulesp]() {
						goto l51
					}
					if !_rules[rulefvalue]() {
						goto l51
					}
					if !_rules[rulecomma]() {
						goto l51
					}
					{
						position53, tokenIndex53 := position, tokenIndex
						if buffer[position] != rune('f') {
							goto l53
						}
						position++
						if buffer[position] != rune('r') {
							goto l53
						}
						position++
						if buffer[position] != rune('o') {
							goto l53
						}
						position++
						if buffer[position] != rune('m') {
							goto l53
						}
						position++
						if buffer[position] != rune('=') {
							goto l53
						}
						position++
						goto l54
					l53:
						position, tokenIndex = position53, tokenIndex53
					}
				l54:
					{
						add(ruleAction17, position)
					}
					if !_rules[ruletimestampfmt]() {
						goto l51
					}
					{
						add(ruleAction18, position)
					}
					if !_rules[rulecomma]() {
						goto l51
					}
					{
						position57, tokenIndex57 := position, tokenIndex
						if buffer[position] != rune('t') {
							goto l57
						}
						position++
						if buffer[position] != rune('o') {
							goto l57
						}
						position++
						if buffer[position] != rune('=') {
							goto l57
						}
						position++
						goto l58
					l57:
						position, tokenIndex = position57, tokenIndex57
					}
				l58:
					if !_rules[rulesp]() {
						goto l51
					}
					{
						add(ruleAction19, position)
					}
					if !_rules[ruletimestampfmt]() {
						goto l51
					}
					{
						add(ruleAction20, position)
					}
					if !_rules[ruleclose]() {
						goto l51
					}
					{
						add(ruleAction21, position)
					}
					goto l7
				l51:
					position, tokenIndex = position7, tokenIndex7
					{
						position62 := position
						if !_rules[ruleIDENT]() {
							goto l5
						}
						add(rulePegText, position62)
					}
					{
						add(ruleAction22, position)
					}
					if !_rules[ruleopen]() {
						goto l5
					}
					if !_rules[ruleallargs]() {
						goto l5
					}
					{
						position64, tokenIndex64 := position, tokenIndex
						if !_rules[rulecomma]() {
							goto l64
						}
						goto l65
					l64:
						position, tokenIndex = position64, tokenIndex64
					}
				l65:
					if !_rules[ruleclose]() {
						goto l5
					}
					{
						add(ruleAction23, position)
					}
				}
			l7:
				add(ruleCall, position6)
			}
			return true
		l5:
			position, tokenIndex = position5, tokenIndex5
			return false
		},
		/* 2 allargs <- <((Call (comma Call)* (comma dargs)?) / dargs / sp)> */
		func() bool {
			position67, tokenIndex67 := position, tokenIndex
			{
				position68 := position
				{
					position69, tokenIndex69 := position, tokenIndex
					if !_rules[ruleCall]() {
						goto l70
					}
				l71:
					{
						position72, tokenIndex72 := position, tokenIndex
						if !_rules[rulecomma]() {
							goto l72
						}
						if !_rules[ruleCall]() {
							goto l72
						}
						goto l71
					l72:
						position, tokenIndex = position72, tokenIndex72
					}
					{
						position73, tokenIndex73 := position, tokenIndex
						if !_rules[rulecomma]() {
							goto l73
						}
						if !_rules[ruledargs]() {
							goto l73
						}
						goto l74
					l73:
						position, tokenIndex = position73, tokenIndex73
					}
				l74:
					goto l69
				l70:
					position, tokenIndex = position69, tokenIndex69
					if !_rules[ruledargs]() {
						goto l75
					}
					goto l69
				l75:
					position, tokenIndex = position69, tokenIndex69
					if !_rules[rulesp]() {
						goto l67
					}
				}
			l69:
				add(ruleallargs, position68)
			}
			return true
		l67:
			position, tokenIndex = position67, tokenIndex67
			return false
		},
		/* 3 fargs <- <(farg (comma fargs)? sp)> */
		func() bool {
			position76, tokenIndex76 := position, tokenIndex
			{
				position77 := position
				{
					position78 := position
					{
						position79, tokenIndex79 := position, tokenIndex
						if !_rules[rulefield]() {
							goto l80
						}
						if !_rules[rulesp]() {
							goto l80
						}
						if buffer[position] != rune('=') {
							goto l80
						}
						position++
						if !_rules[rulesp]() {
							goto l80
						}
						if !_rules[rulefvalue]() {
							goto l80
						}
						goto l79
					l80:
						position, tokenIndex = position79, tokenIndex79
						if !_rules[rulefield]() {
							goto l81
						}
						if !_rules[rulesp]() {
							goto l81
						}
						if !_rules[ruleCOND]() {
							goto l81
						}
						if !_rules[rulesp]() {
							goto l81
						}
						if !_rules[rulefvalue]() {
							goto l81
						}
						goto l79
					l81:
						position, tokenIndex = position79, tokenIndex79
						if !_rules[ruleconditional]() {
							goto l76
						}
					}
				l79:
					add(rulefarg, position78)
				}
				{
					position82, tokenIndex82 := position, tokenIndex
					if !_rules[rulecomma]() {
						goto l82
					}
					if !_rules[rulefargs]() {
						goto l82
					}
					goto l83
				l82:
					position, tokenIndex = position82, tokenIndex82
				}
			l83:
				if !_rules[rulesp]() {
					goto l76
				}
				add(rulefargs, position77)
			}
			return true
		l76:
			position, tokenIndex = position76, tokenIndex76
			return false
		},
		/* 4 farg <- <((field sp '=' sp fvalue) / (field sp COND sp fvalue) / conditional)> */
		nil,
		/* 5 dargs <- <(darg (comma dargs)? sp)> */
		func() bool {
			position85, tokenIndex85 := position, tokenIndex
			{
				position86 := position
				if !_rules[ruledarg]() {
					goto l85
				}
				{
					position87, tokenIndex87 := position, tokenIndex
					if !_rules[rulecomma]() {
						goto l87
					}
					if !_rules[ruledargs]() {
						goto l87
					}
					goto l88
				l87:
					position, tokenIndex = position87, tokenIndex87
				}
			l88:
				if !_rules[rulesp]() {
					goto l85
				}
				add(ruledargs, position86)
			}
			return true
		l85:
			position, tokenIndex = position85, tokenIndex85
			return false
		},
		/* 6 darg <- <((field sp '=' sp dvalue) / (field sp COND sp dvalue) / conditional)> */
		func() bool {
			position89, tokenIndex89 := position, tokenIndex
			{
				position90 := position
				{
					position91, tokenIndex91 := position, tokenIndex
					if !_rules[rulefield]() {
						goto l92
					}
					if !_rules[rulesp]() {
						goto l92
					}
					if buffer[position] != rune('=') {
						goto l92
					}
					position++
					if !_rules[rulesp]() {
						goto l92
					}
					if !_rules[ruledvalue]() {
						goto l92
					}
					goto l91
				l92:
					position, tokenIndex = position91, tokenIndex91
					if !_rules[rulefield]() {
						goto l93
					}
					if !_rules[rulesp]() {
						goto l93
					}
					if !_rules[ruleCOND]() {
						goto l93
					}
					if !_rules[rulesp]() {
						goto l93
					}
					if !_rules[ruledvalue]() {
						goto l93
					}
					goto l91
				l93:
					position, tokenIndex = position91, tokenIndex91
					if !_rules[ruleconditional]() {
						goto l89
					}
				}
			l91:
				add(ruledarg, position90)
			}
			return true
		l89:
			position, tokenIndex = position89, tokenIndex89
			return false
		},
		/* 7 COND <- <(('>' '<' Action24) / ('<' '=' Action25) / ('>' '=' Action26) / ('=' '=' Action27) / ('!' '=' Action28) / ('<' Action29) / ('>' Action30))> */
		func() bool {
			position94, tokenIndex94 := position, tokenIndex
			{
				position95 := position
				{
					position96, tokenIndex96 := position, tokenIndex
					if buffer[position] != rune('>') {
						goto l97
					}
					position++
					if buffer[position] != rune('<') {
						goto l97
					}
					position++
					{
						add(ruleAction24, position)
					}
					goto l96
				l97:
					position, tokenIndex = position96, tokenIndex96
					if buffer[position] != rune('<') {
						goto l99
					}
					position++
					if buffer[position] != rune('=') {
						goto l99
					}
					position++
					{
						add(ruleAction25, position)
					}
					goto l96
				l99:
					position, tokenIndex = position96, tokenIndex96
					if buffer[position] != rune('>') {
						goto l101
					}
					position++
					if buffer[position] != rune('=') {
						goto l101
					}
					position++
					{
						add(ruleAction26, position)
					}
					goto l96
				l101:
					position, tokenIndex = position96, tokenIndex96
					if buffer[position] != rune('=') {
						goto l103
					}
					position++
					if buffer[position] != rune('=') {
						goto l103
					}
					position++
					{
						add(ruleAction27, position)
					}
					goto l96
				l103:
					position, tokenIndex = position96, tokenIndex96
					if buffer[position] != rune('!') {
						goto l105
					}
					position++
					if buffer[position] != rune('=') {
						goto l105
					}
					position++
					{
						add(ruleAction28, position)
					}
					goto l96
				l105:
					position, tokenIndex = position96, tokenIndex96
					if buffer[position] != rune('<') {
						goto l107
					}
					position++
					{
						add(ruleAction29, position)
					}
					goto l96
				l107:
					position, tokenIndex = position96, tokenIndex96
					if buffer[position] != rune('>') {
						goto l94
					}
					position++
					{
						add(ruleAction30, position)
					}
				}
			l96:
				add(ruleCOND, position95)
			}
			return true
		l94:
			position, tokenIndex = position94, tokenIndex94
			return false
		},
		/* 8 conditional <- <(Action31 condint condLT condfield condLT condint Action32)> */
		func() bool {
			position110, tokenIndex110 := position, tokenIndex
			{
				position111 := position
				{
					add(ruleAction31, position)
				}
				if !_rules[rulecondint]() {
					goto l110
				}
				if !_rules[rulecondLT]() {
					goto l110
				}
				{
					position113 := position
					{
						position114 := position
						if !_rules[rulefieldExpr]() {
							goto l110
						}
						add(rulePegText, position114)
					}
					if !_rules[rulesp]() {
						goto l110
					}
					{
						add(ruleAction35, position)
					}
					add(rulecondfield, position113)
				}
				if !_rules[rulecondLT]() {
					goto l110
				}
				if !_rules[rulecondint]() {
					goto l110
				}
				{
					add(ruleAction32, position)
				}
				add(ruleconditional, position111)
			}
			return true
		l110:
			position, tokenIndex = position110, tokenIndex110
			return false
		},
		/* 9 condint <- <(<(('-'? [0-9]* '.' [0-9]+) / '0' / ('-'? [1-9] [0-9]*))> sp Action33)> */
		func() bool {
			position117, tokenIndex117 := position, tokenIndex
			{
				position118 := position
				{
					position119 := position
					{
						position120, tokenIndex120 := position, tokenIndex
						{
							position122, tokenIndex122 := position, tokenIndex
							if buffer[position] != rune('-') {
								goto l122
							}
							position++
							goto l123
						l122:
							position, tokenIndex = position122, tokenIndex122
						}
					l123:
					l124:
						{
							position125, tokenIndex125 := position, tokenIndex
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l125
							}
							position++
							goto l124
						l125:
							position, tokenIndex = position125, tokenIndex125
						}
						if buffer[position] != rune('.') {
							goto l121
						}
						position++
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l121
						}
						position++
					l126:
						{
							position127, tokenIndex127 := position, tokenIndex
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l127
							}
							position++
							goto l126
						l127:
							position, tokenIndex = position127, tokenIndex127
						}
						goto l120
					l121:
						position, tokenIndex = position120, tokenIndex120
						if buffer[position] != rune('0') {
							goto l128
						}
						position++
						goto l120
					l128:
						position, tokenIndex = position120, tokenIndex120
						{
							position129, tokenIndex129 := position, tokenIndex
							if buffer[position] != rune('-') {
								goto l129
							}
							position++
							goto l130
						l129:
							position, tokenIndex = position129, tokenIndex129
						}
					l130:
						if c := buffer[position]; c < rune('1') || c > rune('9') {
							goto l117
						}
						position++
					l131:
						{
							position132, tokenIndex132 := position, tokenIndex
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l132
							}
							position++
							goto l131
						l132:
							position, tokenIndex = position132, tokenIndex132
						}
					}
				l120:
					add(rulePegText, position119)
				}
				if !_rules[rulesp]() {
					goto l117
				}
				{
					add(ruleAction33, position)
				}
				add(rulecondint, position118)
			}
			return true
		l117:
			position, tokenIndex = position117, tokenIndex117
			return false
		},
		/* 10 condLT <- <(<(('<' '=') / '<')> sp Action34)> */
		func() bool {
			position134, tokenIndex134 := position, tokenIndex
			{
				position135 := position
				{
					position136 := position
					{
						position137, tokenIndex137 := position, tokenIndex
						if buffer[position] != rune('<') {
							goto l138
						}
						position++
						if buffer[position] != rune('=') {
							goto l138
						}
						position++
						goto l137
					l138:
						position, tokenIndex = position137, tokenIndex137
						if buffer[position] != rune('<') {
							goto l134
						}
						position++
					}
				l137:
					add(rulePegText, position136)
				}
				if !_rules[rulesp]() {
					goto l134
				}
				{
					add(ruleAction34, position)
				}
				add(rulecondLT, position135)
			}
			return true
		l134:
			position, tokenIndex = position134, tokenIndex134
			return false
		},
		/* 11 condfield <- <(<fieldExpr> sp Action35)> */
		nil,
		/* 12 dvalue <- <(ditem / (lbrack Action36 dlist rbrack Action37))> */
		func() bool {
			position141, tokenIndex141 := position, tokenIndex
			{
				position142 := position
				{
					position143, tokenIndex143 := position, tokenIndex
					if !_rules[ruleditem]() {
						goto l144
					}
					goto l143
				l144:
					position, tokenIndex = position143, tokenIndex143
					if !_rules[rulelbrack]() {
						goto l141
					}
					{
						add(ruleAction36, position)
					}
					if !_rules[ruledlist]() {
						goto l141
					}
					if !_rules[rulerbrack]() {
						goto l141
					}
					{
						add(ruleAction37, position)
					}
				}
			l143:
				add(ruledvalue, position142)
			}
			return true
		l141:
			position, tokenIndex = position141, tokenIndex141
			return false
		},
		/* 13 fvalue <- <(fitem / (lbrack Action38 flist rbrack Action39))> */
		func() bool {
			position147, tokenIndex147 := position, tokenIndex
			{
				position148 := position
				{
					position149, tokenIndex149 := position, tokenIndex
					if !_rules[rulefitem]() {
						goto l150
					}
					goto l149
				l150:
					position, tokenIndex = position149, tokenIndex149
					if !_rules[rulelbrack]() {
						goto l147
					}
					{
						add(ruleAction38, position)
					}
					if !_rules[ruleflist]() {
						goto l147
					}
					if !_rules[rulerbrack]() {
						goto l147
					}
					{
						add(ruleAction39, position)
					}
				}
			l149:
				add(rulefvalue, position148)
			}
			return true
		l147:
			position, tokenIndex = position147, tokenIndex147
			return false
		},
		/* 14 dlist <- <(ditem (comma dlist)?)> */
		func() bool {
			position153, tokenIndex153 := position, tokenIndex
			{
				position154 := position
				if !_rules[ruleditem]() {
					goto l153
				}
				{
					position155, tokenIndex155 := position, tokenIndex
					if !_rules[rulecomma]() {
						goto l155
					}
					if !_rules[ruledlist]() {
						goto l155
					}
					goto l156
				l155:
					position, tokenIndex = position155, tokenIndex155
				}
			l156:
				add(ruledlist, position154)
			}
			return true
		l153:
			position, tokenIndex = position153, tokenIndex153
			return false
		},
		/* 15 flist <- <(fitem (comma flist)?)> */
		func() bool {
			position157, tokenIndex157 := position, tokenIndex
			{
				position158 := position
				if !_rules[rulefitem]() {
					goto l157
				}
				{
					position159, tokenIndex159 := position, tokenIndex
					if !_rules[rulecomma]() {
						goto l159
					}
					if !_rules[ruleflist]() {
						goto l159
					}
					goto l160
				l159:
					position, tokenIndex = position159, tokenIndex159
				}
			l160:
				add(ruleflist, position158)
			}
			return true
		l157:
			position, tokenIndex = position157, tokenIndex157
			return false
		},
		/* 16 ditem <- <(itema / decimal / itemb)> */
		func() bool {
			position161, tokenIndex161 := position, tokenIndex
			{
				position162 := position
				{
					position163, tokenIndex163 := position, tokenIndex
					if !_rules[ruleitema]() {
						goto l164
					}
					goto l163
				l164:
					position, tokenIndex = position163, tokenIndex163
					{
						position166 := position
						{
							position167, tokenIndex167 := position, tokenIndex
							{
								position169 := position
								{
									position170, tokenIndex170 := position, tokenIndex
									if buffer[position] != rune('-') {
										goto l170
									}
									position++
									goto l171
								l170:
									position, tokenIndex = position170, tokenIndex170
								}
							l171:
								if c := buffer[position]; c < rune('0') || c > rune('9') {
									goto l168
								}
								position++
							l172:
								{
									position173, tokenIndex173 := position, tokenIndex
									if c := buffer[position]; c < rune('0') || c > rune('9') {
										goto l173
									}
									position++
									goto l172
								l173:
									position, tokenIndex = position173, tokenIndex173
								}
								{
									position174, tokenIndex174 := position, tokenIndex
									if buffer[position] != rune('.') {
										goto l174
									}
									position++
								l176:
									{
										position177, tokenIndex177 := position, tokenIndex
										if c := buffer[position]; c < rune('0') || c > rune('9') {
											goto l177
										}
										position++
										goto l176
									l177:
										position, tokenIndex = position177, tokenIndex177
									}
									goto l175
								l174:
									position, tokenIndex = position174, tokenIndex174
								}
							l175:
								add(rulePegText, position169)
							}
							{
								add(ruleAction51, position)
							}
							goto l167
						l168:
							position, tokenIndex = position167, tokenIndex167
							{
								position179 := position
								{
									position180, tokenIndex180 := position, tokenIndex
									if buffer[position] != rune('-') {
										goto l180
									}
									position++
									goto l181
								l180:
									position, tokenIndex = position180, tokenIndex180
								}
							l181:
								if buffer[position] != rune('.') {
									goto l165
								}
								position++
								if c := buffer[position]; c < rune('0') || c > rune('9') {
									goto l165
								}
								position++
							l182:
								{
									position183, tokenIndex183 := position, tokenIndex
									if c := buffer[position]; c < rune('0') || c > rune('9') {
										goto l183
									}
									position++
									goto l182
								l183:
									position, tokenIndex = position183, tokenIndex183
								}
								add(rulePegText, position179)
							}
							{
								add(ruleAction52, position)
							}
						}
					l167:
						add(ruledecimal, position166)
					}
					goto l163
				l165:
					position, tokenIndex = position163, tokenIndex163
					if !_rules[ruleitemb]() {
						goto l161
					}
				}
			l163:
				add(ruleditem, position162)
			}
			return true
		l161:
			position, tokenIndex = position161, tokenIndex161
			return false
		},
		/* 17 fitem <- <(itema / float / itemb)> */
		func() bool {
			position185, tokenIndex185 := position, tokenIndex
			{
				position186 := position
				{
					position187, tokenIndex187 := position, tokenIndex
					if !_rules[ruleitema]() {
						goto l188
					}
					goto l187
				l188:
					position, tokenIndex = position187, tokenIndex187
					{
						position190 := position
						{
							position191, tokenIndex191 := position, tokenIndex
							{
								position193 := position
								{
									position194, tokenIndex194 := position, tokenIndex
									if buffer[position] != rune('-') {
										goto l194
									}
									position++
									goto l195
								l194:
									position, tokenIndex = position194, tokenIndex194
								}
							l195:
								if c := buffer[position]; c < rune('0') || c > rune('9') {
									goto l192
								}
								position++
							l196:
								{
									position197, tokenIndex197 := position, tokenIndex
									if c := buffer[position]; c < rune('0') || c > rune('9') {
										goto l197
									}
									position++
									goto l196
								l197:
									position, tokenIndex = position197, tokenIndex197
								}
								{
									position198, tokenIndex198 := position, tokenIndex
									if buffer[position] != rune('.') {
										goto l198
									}
									position++
								l200:
									{
										position201, tokenIndex201 := position, tokenIndex
										if c := buffer[position]; c < rune('0') || c > rune('9') {
											goto l201
										}
										position++
										goto l200
									l201:
										position, tokenIndex = position201, tokenIndex201
									}
									goto l199
								l198:
									position, tokenIndex = position198, tokenIndex198
								}
							l199:
								add(rulePegText, position193)
							}
							{
								add(ruleAction49, position)
							}
							goto l191
						l192:
							position, tokenIndex = position191, tokenIndex191
							{
								position203 := position
								{
									position204, tokenIndex204 := position, tokenIndex
									if buffer[position] != rune('-') {
										goto l204
									}
									position++
									goto l205
								l204:
									position, tokenIndex = position204, tokenIndex204
								}
							l205:
								if buffer[position] != rune('.') {
									goto l189
								}
								position++
								if c := buffer[position]; c < rune('0') || c > rune('9') {
									goto l189
								}
								position++
							l206:
								{
									position207, tokenIndex207 := position, tokenIndex
									if c := buffer[position]; c < rune('0') || c > rune('9') {
										goto l207
									}
									position++
									goto l206
								l207:
									position, tokenIndex = position207, tokenIndex207
								}
								add(rulePegText, position203)
							}
							{
								add(ruleAction50, position)
							}
						}
					l191:
						add(rulefloat, position190)
					}
					goto l187
				l189:
					position, tokenIndex = position187, tokenIndex187
					if !_rules[ruleitemb]() {
						goto l185
					}
				}
			l187:
				add(rulefitem, position186)
			}
			return true
		l185:
			position, tokenIndex = position185, tokenIndex185
			return false
		},
		/* 18 itema <- <(('n' 'u' 'l' 'l' &(comma / (sp close)) Action40) / ('t' 'r' 'u' 'e' &(comma / (sp close)) Action41) / ('f' 'a' 'l' 's' 'e' &(comma / (sp close)) Action42) / (timestampfmt Action43))> */
		func() bool {
			position209, tokenIndex209 := position, tokenIndex
			{
				position210 := position
				{
					position211, tokenIndex211 := position, tokenIndex
					if buffer[position] != rune('n') {
						goto l212
					}
					position++
					if buffer[position] != rune('u') {
						goto l212
					}
					position++
					if buffer[position] != rune('l') {
						goto l212
					}
					position++
					if buffer[position] != rune('l') {
						goto l212
					}
					position++
					{
						position213, tokenIndex213 := position, tokenIndex
						{
							position214, tokenIndex214 := position, tokenIndex
							if !_rules[rulecomma]() {
								goto l215
							}
							goto l214
						l215:
							position, tokenIndex = position214, tokenIndex214
							if !_rules[rulesp]() {
								goto l212
							}
							if !_rules[ruleclose]() {
								goto l212
							}
						}
					l214:
						position, tokenIndex = position213, tokenIndex213
					}
					{
						add(ruleAction40, position)
					}
					goto l211
				l212:
					position, tokenIndex = position211, tokenIndex211
					if buffer[position] != rune('t') {
						goto l217
					}
					position++
					if buffer[position] != rune('r') {
						goto l217
					}
					position++
					if buffer[position] != rune('u') {
						goto l217
					}
					position++
					if buffer[position] != rune('e') {
						goto l217
					}
					position++
					{
						position218, tokenIndex218 := position, tokenIndex
						{
							position219, tokenIndex219 := position, tokenIndex
							if !_rules[rulecomma]() {
								goto l220
							}
							goto l219
						l220:
							position, tokenIndex = position219, tokenIndex219
							if !_rules[rulesp]() {
								goto l217
							}
							if !_rules[ruleclose]() {
								goto l217
							}
						}
					l219:
						position, tokenIndex = position218, tokenIndex218
					}
					{
						add(ruleAction41, position)
					}
					goto l211
				l217:
					position, tokenIndex = position211, tokenIndex211
					if buffer[position] != rune('f') {
						goto l222
					}
					position++
					if buffer[position] != rune('a') {
						goto l222
					}
					position++
					if buffer[position] != rune('l') {
						goto l222
					}
					position++
					if buffer[position] != rune('s') {
						goto l222
					}
					position++
					if buffer[position] != rune('e') {
						goto l222
					}
					position++
					{
						position223, tokenIndex223 := position, tokenIndex
						{
							position224, tokenIndex224 := position, tokenIndex
							if !_rules[rulecomma]() {
								goto l225
							}
							goto l224
						l225:
							position, tokenIndex = position224, tokenIndex224
							if !_rules[rulesp]() {
								goto l222
							}
							if !_rules[ruleclose]() {
								goto l222
							}
						}
					l224:
						position, tokenIndex = position223, tokenIndex223
					}
					{
						add(ruleAction42, position)
					}
					goto l211
				l222:
					position, tokenIndex = position211, tokenIndex211
					if !_rules[ruletimestampfmt]() {
						goto l209
					}
					{
						add(ruleAction43, position)
					}
				}
			l211:
				add(ruleitema, position210)
			}
			return true
		l209:
			position, tokenIndex = position209, tokenIndex209
			return false
		},
		/* 19 itemb <- <((<IDENT> Action44 open allargs comma? close Action45) / (<([a-z] / [A-Z] / [0-9] / '-' / '_' / ':')+> Action46) / (<('"' doublequotedstring '"')> Action47) / (<('\'' singlequotedstring '\'')> Action48))> */
		func() bool {
			position228, tokenIndex228 := position, tokenIndex
			{
				position229 := position
				{
					position230, tokenIndex230 := position, tokenIndex
					{
						position232 := position
						if !_rules[ruleIDENT]() {
							goto l231
						}
						add(rulePegText, position232)
					}
					{
						add(ruleAction44, position)
					}
					if !_rules[ruleopen]() {
						goto l231
					}
					if !_rules[ruleallargs]() {
						goto l231
					}
					{
						position234, tokenIndex234 := position, tokenIndex
						if !_rules[rulecomma]() {
							goto l234
						}
						goto l235
					l234:
						position, tokenIndex = position234, tokenIndex234
					}
				l235:
					if !_rules[ruleclose]() {
						goto l231
					}
					{
						add(ruleAction45, position)
					}
					goto l230
				l231:
					position, tokenIndex = position230, tokenIndex230
					{
						position238 := position
						{
							position241, tokenIndex241 := position, tokenIndex
							if c := buffer[position]; c < rune('a') || c > rune('z') {
								goto l242
							}
							position++
							goto l241
						l242:
							position, tokenIndex = position241, tokenIndex241
							if c := buffer[position]; c < rune('A') || c > rune('Z') {
								goto l243
							}
							position++
							goto l241
						l243:
							position, tokenIndex = position241, tokenIndex241
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l244
							}
							position++
							goto l241
						l244:
							position, tokenIndex = position241, tokenIndex241
							if buffer[position] != rune('-') {
								goto l245
							}
							position++
							goto l241
						l245:
							position, tokenIndex = position241, tokenIndex241
							if buffer[position] != rune('_') {
								goto l246
							}
							position++
							goto l241
						l246:
							position, tokenIndex = position241, tokenIndex241
							if buffer[position] != rune(':') {
								goto l237
							}
							position++
						}
					l241:
					l239:
						{
							position240, tokenIndex240 := position, tokenIndex
							{
								position247, tokenIndex247 := position, tokenIndex
								if c := buffer[position]; c < rune('a') || c > rune('z') {
									goto l248
								}
								position++
								goto l247
							l248:
								position, tokenIndex = position247, tokenIndex247
								if c := buffer[position]; c < rune('A') || c > rune('Z') {
									goto l249
								}
								position++
								goto l247
							l249:
								position, tokenIndex = position247, tokenIndex247
								if c := buffer[position]; c < rune('0') || c > rune('9') {
									goto l250
								}
								position++
								goto l247
							l250:
								position, tokenIndex = position247, tokenIndex247
								if buffer[position] != rune('-') {
									goto l251
								}
								position++
								goto l247
							l251:
								position, tokenIndex = position247, tokenIndex247
								if buffer[position] != rune('_') {
									goto l252
								}
								position++
								goto l247
							l252:
								position, tokenIndex = position247, tokenIndex247
								if buffer[position] != rune(':') {
									goto l240
								}
								position++
							}
						l247:
							goto l239
						l240:
							position, tokenIndex = position240, tokenIndex240
						}
						add(rulePegText, position238)
					}
					{
						add(ruleAction46, position)
					}
					goto l230
				l237:
					position, tokenIndex = position230, tokenIndex230
					{
						position255 := position
						if buffer[position] != rune('"') {
							goto l254
						}
						position++
						if !_rules[ruledoublequotedstring]() {
							goto l254
						}
						if buffer[position] != rune('"') {
							goto l254
						}
						position++
						add(rulePegText, position255)
					}
					{
						add(ruleAction47, position)
					}
					goto l230
				l254:
					position, tokenIndex = position230, tokenIndex230
					{
						position257 := position
						if buffer[position] != rune('\'') {
							goto l228
						}
						position++
						if !_rules[rulesinglequotedstring]() {
							goto l228
						}
						if buffer[position] != rune('\'') {
							goto l228
						}
						position++
						add(rulePegText, position257)
					}
					{
						add(ruleAction48, position)
					}
				}
			l230:
				add(ruleitemb, position229)
			}
			return true
		l228:
			position, tokenIndex = position228, tokenIndex228
			return false
		},
		/* 20 float <- <((<('-'? [0-9]+ ('.' [0-9]*)?)> Action49) / (<('-'? '.' [0-9]+)> Action50))> */
		nil,
		/* 21 decimal <- <((<('-'? [0-9]+ ('.' [0-9]*)?)> Action51) / (<('-'? '.' [0-9]+)> Action52))> */
		nil,
		/* 22 doublequotedstring <- <(('\\' '"') / ('\\' '\\') / ('\\' 'n') / ('\\' 't') / (!('"' / '\\') .))*> */
		func() bool {
			{
				position262 := position
			l263:
				{
					position264, tokenIndex264 := position, tokenIndex
					{
						position265, tokenIndex265 := position, tokenIndex
						if buffer[position] != rune('\\') {
							goto l266
						}
						position++
						if buffer[position] != rune('"') {
							goto l266
						}
						position++
						goto l265
					l266:
						position, tokenIndex = position265, tokenIndex265
						if buffer[position] != rune('\\') {
							goto l267
						}
						position++
						if buffer[position] != rune('\\') {
							goto l267
						}
						position++
						goto l265
					l267:
						position, tokenIndex = position265, tokenIndex265
						if buffer[position] != rune('\\') {
							goto l268
						}
						position++
						if buffer[position] != rune('n') {
							goto l268
						}
						position++
						goto l265
					l268:
						position, tokenIndex = position265, tokenIndex265
						if buffer[position] != rune('\\') {
							goto l269
						}
						position++
						if buffer[position] != rune('t') {
							goto l269
						}
						position++
						goto l265
					l269:
						position, tokenIndex = position265, tokenIndex265
						{
							position270, tokenIndex270 := position, tokenIndex
							{
								position271, tokenIndex271 := position, tokenIndex
								if buffer[position] != rune('"') {
									goto l272
								}
								position++
								goto l271
							l272:
								position, tokenIndex = position271, tokenIndex271
								if buffer[position] != rune('\\') {
									goto l270
								}
								position++
							}
						l271:
							goto l264
						l270:
							position, tokenIndex = position270, tokenIndex270
						}
						if !matchDot() {
							goto l264
						}
					}
				l265:
					goto l263
				l264:
					position, tokenIndex = position264, tokenIndex264
				}
				add(ruledoublequotedstring, position262)
			}
			return true
		},
		/* 23 singlequotedstring <- <(('\\' '\'') / ('\\' '\\') / ('\\' 'n') / ('\\' 't') / (!('\'' / '\\') .))*> */
		func() bool {
			{
				position274 := position
			l275:
				{
					position276, tokenIndex276 := position, tokenIndex
					{
						position277, tokenIndex277 := position, tokenIndex
						if buffer[position] != rune('\\') {
							goto l278
						}
						position++
						if buffer[position] != rune('\'') {
							goto l278
						}
						position++
						goto l277
					l278:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune('\\') {
							goto l279
						}
						position++
						if buffer[position] != rune('\\') {
							goto l279
						}
						position++
						goto l277
					l279:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune('\\') {
							goto l280
						}
						position++
						if buffer[position] != rune('n') {
							goto l280
						}
						position++
						goto l277
					l280:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune('\\') {
							goto l281
						}
						position++
						if buffer[position] != rune('t') {
							goto l281
						}
						position++
						goto l277
					l281:
						position, tokenIndex = position277, tokenIndex277
						{
							position282, tokenIndex282 := position, tokenIndex
							{
								position283, tokenIndex283 := position, tokenIndex
								if buffer[position] != rune('\'') {
									goto l284
								}
								position++
								goto l283
							l284:
								position, tokenIndex = position283, tokenIndex283
								if buffer[position] != rune('\\') {
									goto l282
								}
								position++
							}
						l283:
							goto l276
						l282:
							position, tokenIndex = position282, tokenIndex282
						}
						if !matchDot() {
							goto l276
						}
					}
				l277:
					goto l275
				l276:
					position, tokenIndex = position276, tokenIndex276
				}
				add(rulesinglequotedstring, position274)
			}
			return true
		},
		/* 24 fieldExpr <- <(([a-z] / [A-Z] / '_') ([a-z] / [A-Z] / [0-9] / '_' / '-')*)> */
		func() bool {
			position285, tokenIndex285 := position, tokenIndex
			{
				position286 := position
				{
					position287, tokenIndex287 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l288
					}
					position++
					goto l287
				l288:
					position, tokenIndex = position287, tokenIndex287
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l289
					}
					position++
					goto l287
				l289:
					position, tokenIndex = position287, tokenIndex287
					if buffer[position] != rune('_') {
						goto l285
					}
					position++
				}
			l287:
			l290:
				{
					position291, tokenIndex291 := position, tokenIndex
					{
						position292, tokenIndex292 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l293
						}
						position++
						goto l292
					l293:
						position, tokenIndex = position292, tokenIndex292
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l294
						}
						position++
						goto l292
					l294:
						position, tokenIndex = position292, tokenIndex292
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l295
						}
						position++
						goto l292
					l295:
						position, tokenIndex = position292, tokenIndex292
						if buffer[position] != rune('_') {
							goto l296
						}
						position++
						goto l292
					l296:
						position, tokenIndex = position292, tokenIndex292
						if buffer[position] != rune('-') {
							goto l291
						}
						position++
					}
				l292:
					goto l290
				l291:
					position, tokenIndex = position291, tokenIndex291
				}
				add(rulefieldExpr, position286)
			}
			return true
		l285:
			position, tokenIndex = position285, tokenIndex285
			return false
		},
		/* 25 field <- <(<(fieldExpr / reserved)> Action53)> */
		func() bool {
			position297, tokenIndex297 := position, tokenIndex
			{
				position298 := position
				{
					position299 := position
					{
						position300, tokenIndex300 := position, tokenIndex
						if !_rules[rulefieldExpr]() {
							goto l301
						}
						goto l300
					l301:
						position, tokenIndex = position300, tokenIndex300
						{
							position302 := position
							{
								position303, tokenIndex303 := position, tokenIndex
								if buffer[position] != rune('_') {
									goto l304
								}
								position++
								if buffer[position] != rune('r') {
									goto l304
								}
								position++
								if buffer[position] != rune('o') {
									goto l304
								}
								position++
								if buffer[position] != rune('w') {
									goto l304
								}
								position++
								goto l303
							l304:
								position, tokenIndex = position303, tokenIndex303
								if buffer[position] != rune('_') {
									goto l305
								}
								position++
								if buffer[position] != rune('c') {
									goto l305
								}
								position++
								if buffer[position] != rune('o') {
									goto l305
								}
								position++
								if buffer[position] != rune('l') {
									goto l305
								}
								position++
								goto l303
							l305:
								position, tokenIndex = position303, tokenIndex303
								if buffer[position] != rune('_') {
									goto l306
								}
								position++
								if buffer[position] != rune('s') {
									goto l306
								}
								position++
								if buffer[position] != rune('t') {
									goto l306
								}
								position++
								if buffer[position] != rune('a') {
									goto l306
								}
								position++
								if buffer[position] != rune('r') {
									goto l306
								}
								position++
								if buffer[position] != rune('t') {
									goto l306
								}
								position++
								goto l303
							l306:
								position, tokenIndex = position303, tokenIndex303
								if buffer[position] != rune('_') {
									goto l307
								}
								position++
								if buffer[position] != rune('e') {
									goto l307
								}
								position++
								if buffer[position] != rune('n') {
									goto l307
								}
								position++
								if buffer[position] != rune('d') {
									goto l307
								}
								position++
								goto l303
							l307:
								position, tokenIndex = position303, tokenIndex303
								if buffer[position] != rune('_') {
									goto l308
								}
								position++
								if buffer[position] != rune('t') {
									goto l308
								}
								position++
								if buffer[position] != rune('i') {
									goto l308
								}
								position++
								if buffer[position] != rune('m') {
									goto l308
								}
								position++
								if buffer[position] != rune('e') {
									goto l308
								}
								position++
								if buffer[position] != rune('s') {
									goto l308
								}
								position++
								if buffer[position] != rune('t') {
									goto l308
								}
								position++
								if buffer[position] != rune('a') {
									goto l308
								}
								position++
								if buffer[position] != rune('m') {
									goto l308
								}
								position++
								if buffer[position] != rune('p') {
									goto l308
								}
								position++
								goto l303
							l308:
								position, tokenIndex = position303, tokenIndex303
								if buffer[position] != rune('_') {
									goto l297
								}
								position++
								if buffer[position] != rune('f') {
									goto l297
								}
								position++
								if buffer[position] != rune('i') {
									goto l297
								}
								position++
								if buffer[position] != rune('e') {
									goto l297
								}
								position++
								if buffer[position] != rune('l') {
									goto l297
								}
								position++
								if buffer[position] != rune('d') {
									goto l297
								}
								position++
							}
						l303:
							add(rulereserved, position302)
						}
					}
				l300:
					add(rulePegText, position299)
				}
				{
					add(ruleAction53, position)
				}
				add(rulefield, position298)
			}
			return true
		l297:
			position, tokenIndex = position297, tokenIndex297
			return false
		},
		/* 26 reserved <- <(('_' 'r' 'o' 'w') / ('_' 'c' 'o' 'l') / ('_' 's' 't' 'a' 'r' 't') / ('_' 'e' 'n' 'd') / ('_' 't' 'i' 'm' 'e' 's' 't' 'a' 'm' 'p') / ('_' 'f' 'i' 'e' 'l' 'd'))> */
		nil,
		/* 27 posfield <- <(<fieldExpr> Action54)> */
		func() bool {
			position311, tokenIndex311 := position, tokenIndex
			{
				position312 := position
				{
					position313 := position
					if !_rules[rulefieldExpr]() {
						goto l311
					}
					add(rulePegText, position313)
				}
				{
					add(ruleAction54, position)
				}
				add(ruleposfield, position312)
			}
			return true
		l311:
			position, tokenIndex = position311, tokenIndex311
			return false
		},
		/* 28 uint <- <(([1-9] [0-9]*) / '0')> */
		func() bool {
			position315, tokenIndex315 := position, tokenIndex
			{
				position316 := position
				{
					position317, tokenIndex317 := position, tokenIndex
					if c := buffer[position]; c < rune('1') || c > rune('9') {
						goto l318
					}
					position++
				l319:
					{
						position320, tokenIndex320 := position, tokenIndex
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l320
						}
						position++
						goto l319
					l320:
						position, tokenIndex = position320, tokenIndex320
					}
					goto l317
				l318:
					position, tokenIndex = position317, tokenIndex317
					if buffer[position] != rune('0') {
						goto l315
					}
					position++
				}
			l317:
				add(ruleuint, position316)
			}
			return true
		l315:
			position, tokenIndex = position315, tokenIndex315
			return false
		},
		/* 29 col <- <((<uint> Action55) / (<('\'' singlequotedstring '\'')> Action56) / (<('"' doublequotedstring '"')> Action57))> */
		func() bool {
			position321, tokenIndex321 := position, tokenIndex
			{
				position322 := position
				{
					position323, tokenIndex323 := position, tokenIndex
					{
						position325 := position
						if !_rules[ruleuint]() {
							goto l324
						}
						add(rulePegText, position325)
					}
					{
						add(ruleAction55, position)
					}
					goto l323
				l324:
					position, tokenIndex = position323, tokenIndex323
					{
						position328 := position
						if buffer[position] != rune('\'') {
							goto l327
						}
						position++
						if !_rules[rulesinglequotedstring]() {
							goto l327
						}
						if buffer[position] != rune('\'') {
							goto l327
						}
						position++
						add(rulePegText, position328)
					}
					{
						add(ruleAction56, position)
					}
					goto l323
				l327:
					position, tokenIndex = position323, tokenIndex323
					{
						position330 := position
						if buffer[position] != rune('"') {
							goto l321
						}
						position++
						if !_rules[ruledoublequotedstring]() {
							goto l321
						}
						if buffer[position] != rune('"') {
							goto l321
						}
						position++
						add(rulePegText, position330)
					}
					{
						add(ruleAction57, position)
					}
				}
			l323:
				add(rulecol, position322)
			}
			return true
		l321:
			position, tokenIndex = position321, tokenIndex321
			return false
		},
		/* 30 row <- <((<uint> Action58) / (<('\'' singlequotedstring '\'')> Action59) / (<('"' doublequotedstring '"')> Action60))> */
		nil,
		/* 31 open <- <('(' sp)> */
		func() bool {
			position333, tokenIndex333 := position, tokenIndex
			{
				position334 := position
				if buffer[position] != rune('(') {
					goto l333
				}
				position++
				if !_rules[rulesp]() {
					goto l333
				}
				add(ruleopen, position334)
			}
			return true
		l333:
			position, tokenIndex = position333, tokenIndex333
			return false
		},
		/* 32 close <- <(')' sp)> */
		func() bool {
			position335, tokenIndex335 := position, tokenIndex
			{
				position336 := position
				if buffer[position] != rune(')') {
					goto l335
				}
				position++
				if !_rules[rulesp]() {
					goto l335
				}
				add(ruleclose, position336)
			}
			return true
		l335:
			position, tokenIndex = position335, tokenIndex335
			return false
		},
		/* 33 sp <- <(' ' / '\t' / '\n')*> */
		func() bool {
			{
				position338 := position
			l339:
				{
					position340, tokenIndex340 := position, tokenIndex
					{
						position341, tokenIndex341 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l342
						}
						position++
						goto l341
					l342:
						position, tokenIndex = position341, tokenIndex341
						if buffer[position] != rune('\t') {
							goto l343
						}
						position++
						goto l341
					l343:
						position, tokenIndex = position341, tokenIndex341
						if buffer[position] != rune('\n') {
							goto l340
						}
						position++
					}
				l341:
					goto l339
				l340:
					position, tokenIndex = position340, tokenIndex340
				}
				add(rulesp, position338)
			}
			return true
		},
		/* 34 comma <- <(sp ',' sp)> */
		func() bool {
			position344, tokenIndex344 := position, tokenIndex
			{
				position345 := position
				if !_rules[rulesp]() {
					goto l344
				}
				if buffer[position] != rune(',') {
					goto l344
				}
				position++
				if !_rules[rulesp]() {
					goto l344
				}
				add(rulecomma, position345)
			}
			return true
		l344:
			position, tokenIndex = position344, tokenIndex344
			return false
		},
		/* 35 lbrack <- <('[' sp)> */
		func() bool {
			position346, tokenIndex346 := position, tokenIndex
			{
				position347 := position
				if buffer[position] != rune('[') {
					goto l346
				}
				position++
				if !_rules[rulesp]() {
					goto l346
				}
				add(rulelbrack, position347)
			}
			return true
		l346:
			position, tokenIndex = position346, tokenIndex346
			return false
		},
		/* 36 rbrack <- <(sp ']' sp)> */
		func() bool {
			position348, tokenIndex348 := position, tokenIndex
			{
				position349 := position
				if !_rules[rulesp]() {
					goto l348
				}
				if buffer[position] != rune(']') {
					goto l348
				}
				position++
				if !_rules[rulesp]() {
					goto l348
				}
				add(rulerbrack, position349)
			}
			return true
		l348:
			position, tokenIndex = position348, tokenIndex348
			return false
		},
		/* 37 IDENT <- <(([a-z] / [A-Z]) ([a-z] / [A-Z] / [0-9])*)> */
		func() bool {
			position350, tokenIndex350 := position, tokenIndex
			{
				position351 := position
				{
					position352, tokenIndex352 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l353
					}
					position++
					goto l352
				l353:
					position, tokenIndex = position352, tokenIndex352
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l350
					}
					position++
				}
			l352:
			l354:
				{
					position355, tokenIndex355 := position, tokenIndex
					{
						position356, tokenIndex356 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l357
						}
						position++
						goto l356
					l357:
						position, tokenIndex = position356, tokenIndex356
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l358
						}
						position++
						goto l356
					l358:
						position, tokenIndex = position356, tokenIndex356
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l355
						}
						position++
					}
				l356:
					goto l354
				l355:
					position, tokenIndex = position355, tokenIndex355
				}
				add(ruleIDENT, position351)
			}
			return true
		l350:
			position, tokenIndex = position350, tokenIndex350
			return false
		},
		/* 38 timestampbasicfmt <- <([0-9] [0-9] [0-9] [0-9] '-' ('0' / '1') [0-9] '-' [0-3] [0-9] 'T' [0-9] [0-9] ':' [0-9] [0-9])> */
		func() bool {
			position359, tokenIndex359 := position, tokenIndex
			{
				position360 := position
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if buffer[position] != rune('-') {
					goto l359
				}
				position++
				{
					position361, tokenIndex361 := position, tokenIndex
					if buffer[position] != rune('0') {
						goto l362
					}
					position++
					goto l361
				l362:
					position, tokenIndex = position361, tokenIndex361
					if buffer[position] != rune('1') {
						goto l359
					}
					position++
				}
			l361:
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if buffer[position] != rune('-') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('3') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if buffer[position] != rune('T') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if buffer[position] != rune(':') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				if c := buffer[position]; c < rune('0') || c > rune('9') {
					goto l359
				}
				position++
				add(ruletimestampbasicfmt, position360)
			}
			return true
		l359:
			position, tokenIndex = position359, tokenIndex359
			return false
		},
		/* 39 timestampfmt <- <(('"' <timestampbasicfmt> '"') / ('\'' <timestampbasicfmt> '\'') / <timestampbasicfmt>)> */
		func() bool {
			position363, tokenIndex363 := position, tokenIndex
			{
				position364 := position
				{
					position365, tokenIndex365 := position, tokenIndex
					if buffer[position] != rune('"') {
						goto l366
					}
					position++
					{
						position367 := position
						if !_rules[ruletimestampbasicfmt]() {
							goto l366
						}
						add(rulePegText, position367)
					}
					if buffer[position] != rune('"') {
						goto l366
					}
					position++
					goto l365
				l366:
					position, tokenIndex = position365, tokenIndex365
					if buffer[position] != rune('\'') {
						goto l368
					}
					position++
					{
						position369 := position
						if !_rules[ruletimestampbasicfmt]() {
							goto l368
						}
						add(rulePegText, position369)
					}
					if buffer[position] != rune('\'') {
						goto l368
					}
					position++
					goto l365
				l368:
					position, tokenIndex = position365, tokenIndex365
					{
						position370 := position
						if !_rules[ruletimestampbasicfmt]() {
							goto l363
						}
						add(rulePegText, position370)
					}
				}
			l365:
				add(ruletimestampfmt, position364)
			}
			return true
		l363:
			position, tokenIndex = position363, tokenIndex363
			return false
		},
		/* 40 timestamp <- <(<timestampfmt> Action61)> */
		nil,
		/* 42 Action0 <- <{p.startCall("Set")}> */
		nil,
		/* 43 Action1 <- <{p.endCall()}> */
		nil,
		/* 44 Action2 <- <{p.startCall("SetRowAttrs")}> */
		nil,
		/* 45 Action3 <- <{p.endCall()}> */
		nil,
		/* 46 Action4 <- <{p.startCall("SetColumnAttrs")}> */
		nil,
		/* 47 Action5 <- <{p.endCall()}> */
		nil,
		/* 48 Action6 <- <{p.startCall("Clear")}> */
		nil,
		/* 49 Action7 <- <{p.endCall()}> */
		nil,
		/* 50 Action8 <- <{p.startCall("ClearRow")}> */
		nil,
		/* 51 Action9 <- <{p.endCall()}> */
		nil,
		/* 52 Action10 <- <{p.startCall("Store")}> */
		nil,
		/* 53 Action11 <- <{p.endCall()}> */
		nil,
		/* 54 Action12 <- <{p.startCall("TopN")}> */
		nil,
		/* 55 Action13 <- <{p.endCall()}> */
		nil,
		/* 56 Action14 <- <{p.startCall("Rows")}> */
		nil,
		/* 57 Action15 <- <{p.endCall()}> */
		nil,
		/* 58 Action16 <- <{p.startCall("Range")}> */
		nil,
		/* 59 Action17 <- <{p.addField("from")}> */
		nil,
		/* 60 Action18 <- <{p.addVal(buffer[begin:end])}> */
		nil,
		/* 61 Action19 <- <{p.addField("to")}> */
		nil,
		/* 62 Action20 <- <{p.addVal(buffer[begin:end])}> */
		nil,
		/* 63 Action21 <- <{p.endCall()}> */
		nil,
		nil,
		/* 65 Action22 <- <{ p.startCall(buffer[begin:end] ) }> */
		nil,
		/* 66 Action23 <- <{ p.endCall() }> */
		nil,
		/* 67 Action24 <- <{ p.addBTWN() }> */
		nil,
		/* 68 Action25 <- <{ p.addLTE() }> */
		nil,
		/* 69 Action26 <- <{ p.addGTE() }> */
		nil,
		/* 70 Action27 <- <{ p.addEQ() }> */
		nil,
		/* 71 Action28 <- <{ p.addNEQ() }> */
		nil,
		/* 72 Action29 <- <{ p.addLT() }> */
		nil,
		/* 73 Action30 <- <{ p.addGT() }> */
		nil,
		/* 74 Action31 <- <{p.startConditional()}> */
		nil,
		/* 75 Action32 <- <{p.endConditional()}> */
		nil,
		/* 76 Action33 <- <{p.condAdd(buffer[begin:end])}> */
		nil,
		/* 77 Action34 <- <{p.condAdd(buffer[begin:end])}> */
		nil,
		/* 78 Action35 <- <{p.condAdd(buffer[begin:end])}> */
		nil,
		/* 79 Action36 <- <{ p.startList() }> */
		nil,
		/* 80 Action37 <- <{ p.endList() }> */
		nil,
		/* 81 Action38 <- <{ p.startList() }> */
		nil,
		/* 82 Action39 <- <{ p.endList() }> */
		nil,
		/* 83 Action40 <- <{ p.addVal(nil) }> */
		nil,
		/* 84 Action41 <- <{ p.addVal(true) }> */
		nil,
		/* 85 Action42 <- <{ p.addVal(false) }> */
		nil,
		/* 86 Action43 <- <{ p.addVal(buffer[begin:end]) }> */
		nil,
		/* 87 Action44 <- <{ p.startCall(string(_buffer[begin:end])) }> */
		nil,
		/* 88 Action45 <- <{ p.addVal(p.endCall()) }> */
		nil,
		/* 89 Action46 <- <{ p.addVal(string(_buffer[begin:end])) }> */
		nil,
		/* 90 Action47 <- <{ p.addVal(string(_buffer[begin:end])) }> */
		nil,
		/* 91 Action48 <- <{ p.addVal(string(_buffer[begin:end])) }> */
		nil,
		/* 92 Action49 <- <{ p.addNumVal(buffer[begin:end], true) }> */
		nil,
		/* 93 Action50 <- <{ p.addNumVal(buffer[begin:end], true) }> */
		nil,
		/* 94 Action51 <- <{ p.addNumVal(buffer[begin:end], false) }> */
		nil,
		/* 95 Action52 <- <{ p.addNumVal(buffer[begin:end], false) }> */
		nil,
		/* 96 Action53 <- <{ p.addField(buffer[begin:end]) }> */
		nil,
		/* 97 Action54 <- <{ p.addPosStr("_field", buffer[begin:end]) }> */
		nil,
		/* 98 Action55 <- <{p.addPosNum("_col", buffer[begin:end])}> */
		nil,
		/* 99 Action56 <- <{p.addPosStr("_col", buffer[begin:end])}> */
		nil,
		/* 100 Action57 <- <{p.addPosStr("_col", buffer[begin:end])}> */
		nil,
		/* 101 Action58 <- <{p.addPosNum("_row", buffer[begin:end])}> */
		nil,
		/* 102 Action59 <- <{p.addPosStr("_row", buffer[begin:end])}> */
		nil,
		/* 103 Action60 <- <{p.addPosStr("_row", buffer[begin:end])}> */
		nil,
		/* 104 Action61 <- <{p.addPosStr("_timestamp", buffer[begin:end])}> */
		nil,
	}
	p.rules = _rules
}
