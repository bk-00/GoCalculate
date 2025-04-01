package main

import (
	"fmt"
	"go/parser"
	"html/template"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type PageVariables struct {
	ArithmeticEquation string
	IsValid            bool
	Result             string
}

// Node represents a binary tree node for an expression
type Node struct {
	Value string
	Left  *Node
	Right *Node
}

func main() {
	// Handle the root URL
	http.HandleFunc("/", calculatorHandler)

	// Start the server
	fmt.Println("Server started at http://localhost:8010")
	http.ListenAndServe(":8011", nil)
}

// Calculator handler for the web form
func calculatorHandler(w http.ResponseWriter, r *http.Request) {
	// Set initial values for the page
	pageVariables := PageVariables{
		ArithmeticEquation: "",
		IsValid:            false,
		Result:             "",
	}

	// If the form was submitted
	if r.Method == http.MethodPost {
		// Parse form data
		r.ParseForm()
		arithEq := r.FormValue("arithmetic_equation")

		// Perform the calculation
		isValid, result := performArithmeticCalculation(arithEq)

		// Update the pageVariables with input values and result
		pageVariables.Result = result
		pageVariables.IsValid = isValid
		pageVariables.ArithmeticEquation = arithEq
	}

	// Render HTML template with variables
	tmpl, err := template.New("calculator").Parse(`
	<!DOCTYPE html>
	<html>
	<head>
		<title>Arithmetic Calculator</title>
		<style>
			#rule{
				display: inline-block;
				background-color: bisque;
				font-family: Arial;
				font-size: 13px;
				line-height: 0.8;
				padding: 15px;
			}
			.ExpressionInput{
				display: flex;
				margin-top: 15px;
			}
		</style>
	</head>
	<body>
		<h1>Arithmetic Calculator</h1>
		<div id="rule">
			<p>Rules: </p>
			<p>1. Accept operation for Addition, Substraction, Multiplication, Division</p>
			<p>2. Expression should only contain numbers, decimal point, +, -, *, /, (, )</p>
			<p>3. Negative and decimal values are allowed to be entered directly, eg. -1+-2.1, 1.5/-2</p>
			<p>4. Multiplication can be done as eg. 1*-2, 1(-2)</p>
			<p>5. Enter the expression as eg. 1 + ( 2.5 * 3 - ( 4 / 5.7 ) - 6.01 ) + 7</p>
		</div>
		<form method="POST" class="ExpressionInput">
			<input type="text" name="arithmetic_equation" maxlength="100" size="60" value="{{.ArithmeticEquation}}" required>
			<input type="submit" value="Calculate">
		</form>
		<p style="font-weight:bold; color:{{if.IsValid}}green {{else}}red{{end}};">
			{{if.IsValid}}Valid Expression{{else}}Invalid Expression{{end}}
		</p>
		<h2>Result: {{.Result}}</h2>
	</body>
	</html>
	`)

	if err != nil {
		http.Error(w, "Failed to load template", http.StatusInternalServerError)
		return
	}

	// Render the template with the data
	tmpl.Execute(w, pageVariables)
}

func performArithmeticCalculation(Expr string) (bool, string) {
	if validateArithmeticExpression(Expr) {
		tokens := tokenizeExpression(Expr)
		tree := buildTree(tokens)
		result := roundFloat(evaluate(tree), 4)

		return true, strconv.FormatFloat(result, 'f', -1, 64)
	} else {
		return false, ""
	}
}

func validateArithmeticExpression(Expr string) bool {
	Expr = strings.ReplaceAll(Expr, " ", "")

	re := regexp.MustCompile(`^[0-9\+\-\*/\(\)\s.]+$`)

	if !re.MatchString(Expr) {
		return false
	}

	_, err := parser.ParseExpr(Expr)
	return err == nil
}

func tokenizeExpression(expression string) []string {
	var tokens []string
	var number strings.Builder
	var prevToken string

	for i, ch := range expression {
		switch {
		case unicode.IsDigit(ch) || ch == '.': // If digit, accumulate it
			number.WriteRune(ch)
		case ch == '+' || ch == '-' || ch == '*' || ch == '/': // If operator
			if number.Len() > 0 {
				tokens = append(tokens, number.String())
				number.Reset()
			}

			// Handle negative numbers (unary minus)
			if ch == '-' {
				if i == 0 || prevToken == "(" || prevToken == "" || prevToken == "+" || prevToken == "-" || prevToken == "*" || prevToken == "/" {
					number.WriteRune(ch)
					continue
				}
			}

			// Store operator separately
			tokens = append(tokens, string(ch))
			prevToken = string(ch)

		// If parenthesis
		case ch == '(' || ch == ')':
			if number.Len() > 0 {
				tokens = append(tokens, number.String())
				number.Reset()
			}

			// Check for implicit multiplication: number followed by '('
			if ch == '(' && len(tokens) > 0 {
				lastToken := tokens[len(tokens)-1]
				if unicode.IsDigit(rune(lastToken[len(lastToken)-1])) || lastToken == ")" {
					tokens = append(tokens, "*")
				}
			}

			tokens = append(tokens, string(ch)) // Store parentheses separately
			prevToken = string(ch)
		case ch == ' ': // Ignore spaces
			continue
		default:
			fmt.Println("Unexpected character:", string(ch))
		}
	}

	// Add last accumulated number
	if number.Len() > 0 {
		tokens = append(tokens, number.String())
	}

	return tokens
}

func buildTree(tokens []string) *Node {
	if len(tokens) == 0 {
		return nil
	}

	precedence := map[string]int{
		"+": 1, "-": 1,
		"*": 2, "/": 2,
	}

	var build func(int, int) *Node
	build = func(start, end int) *Node {
		if start > end {
			return nil
		}

		// If single number, return as node
		if start == end {
			if _, err := strconv.ParseFloat(tokens[start], 64); err == nil {
				return &Node{Value: tokens[start]}
			}
		}

		// Handle unary minus (e.g., "-2")
		if tokens[start] == "-" && start+1 <= end {
			if _, err := strconv.ParseFloat(tokens[start+1], 64); err == nil {
				return &Node{
					Value: tokens[start] + tokens[start+1], // "-2"
				}
			}
		}

		// Handle surrounding parentheses
		if tokens[start] == "(" && tokens[end] == ")" {
			return build(start+1, end-1)
		}

		// Find the lowest precedence operator (outside of parentheses)
		minPrecedence := 3
		opIndex := -1
		parens := 0

		for i := start; i <= end; i++ {
			switch tokens[i] {
			case "(":
				parens++
			case ")":
				parens--
			default:
				if parens == 0 {
					if prec, exists := precedence[tokens[i]]; exists {
						if prec <= minPrecedence {
							minPrecedence = prec
							opIndex = i
						}
					}
				}
			}
		}

		// If an operator was found, split at that point
		if opIndex != -1 {
			return &Node{
				Value: tokens[opIndex],
				Left:  build(start, opIndex-1),
				Right: build(opIndex+1, end),
			}
		}

		return nil
	}

	return build(0, len(tokens)-1)
}

func evaluate(node *Node) float64 {
	if node == nil {
		return 0
	}

	// If it's a number, return it
	if node.Left == nil && node.Right == nil {
		num, err := strconv.ParseFloat(node.Value, 64)
		if err != nil {
			panic("Invalid number: " + node.Value)
		}
		return num
	}

	// Handle unary minus case
	if node.Left == nil && node.Value == "-" {
		return -evaluate(node.Right)
	}

	// Evaluate left and right subtrees
	leftVal := evaluate(node.Left)
	rightVal := evaluate(node.Right)

	// Perform the operation
	switch node.Value {
	case "+":
		return leftVal + rightVal
	case "-":
		return leftVal - rightVal
	case "*":
		return leftVal * rightVal
	case "/":
		if rightVal == 0 {
			panic("division by zero")
		}
		return leftVal / rightVal
	default:
		panic("unknown operator: " + node.Value)
	}
}

func roundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
