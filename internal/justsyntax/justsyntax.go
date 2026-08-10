package justsyntax

import (
	"strconv"
	"strings"
)

// Import is a syntactically valid top-level Just import declaration.
type Import struct {
	Optional bool
	Path     string
}

// Recipes returns the names of top-level recipes declared by a Just source.
func Recipes(value string) map[string]bool {
	result := map[string]bool{}
	for _, line := range strings.Split(value, "\n") {
		if name, ok := recipeName(line); ok {
			result[name] = true
		}
	}
	return result
}

func recipeName(line string) (string, bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", false
	}
	if line[0] == '@' {
		line = line[1:]
	}
	end := 0
	for end < len(line) && identifierCharacter(line[end], end == 0) {
		end++
	}
	if end == 0 {
		return "", false
	}
	name := line[:end]
	var quote byte
	escaped := false
	var round, square, curly int
	for index := end; index < len(line); index++ {
		character := line[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '#':
			return "", false
		case '(':
			round++
		case ')':
			if round > 0 {
				round--
			}
		case '[':
			square++
		case ']':
			if square > 0 {
				square--
			}
		case '{':
			curly++
		case '}':
			if curly > 0 {
				curly--
			}
		case ':':
			if round == 0 && square == 0 && curly == 0 {
				if index+1 < len(line) && line[index+1] == '=' {
					return "", false
				}
				return name, true
			}
		}
	}
	return "", false
}

func identifierCharacter(character byte, first bool) bool {
	if character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' {
		return true
	}
	return !first && (character == '-' || character >= '0' && character <= '9')
}

// Imports returns syntactically valid top-level Just import declarations.
func Imports(value string) []Import {
	var result []Import
	for _, line := range strings.Split(value, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		optional := false
		remainder := ""
		switch {
		case strings.HasPrefix(line, "import? ") || strings.HasPrefix(line, "import?\t"):
			optional = true
			remainder = strings.TrimSpace(line[len("import?"):])
		case strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "import\t"):
			remainder = strings.TrimSpace(line[len("import"):])
		default:
			continue
		}
		if len(remainder) < 2 || remainder[0] != '\'' && remainder[0] != '"' {
			continue
		}
		quote := remainder[0]
		end := 1
		escaped := false
		for ; end < len(remainder); end++ {
			character := remainder[end]
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if character == quote {
				break
			}
		}
		if end == len(remainder) {
			continue
		}
		trailing := strings.TrimSpace(remainder[end+1:])
		if trailing != "" && !strings.HasPrefix(trailing, "#") {
			continue
		}
		importPath := remainder[1:end]
		if quote == '"' {
			decoded, err := strconv.Unquote(remainder[:end+1])
			if err != nil {
				continue
			}
			importPath = decoded
		}
		result = append(result, Import{Optional: optional, Path: importPath})
	}
	return result
}
