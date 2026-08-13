package main

import (
	"fmt"
	"strconv"
	"strings"
)

// toolMap traduce capacidades canónicas al identificador de cada runtime.
// Es el ÚNICO punto que hay que tocar si un runtime renombra sus herramientas.
var toolMap = map[string]struct {
	Claude  []string
	Copilot []string
}{
	"read":   {Claude: []string{"Read"}, Copilot: []string{"search"}},
	"edit":   {Claude: []string{"Read", "Edit", "Write"}, Copilot: []string{"edit"}},
	"bash":   {Claude: []string{"Bash"}, Copilot: []string{"runCommands"}},
	"search": {Claude: []string{"Grep", "Glob"}, Copilot: []string{"search"}},
}

const noticeFmt = "<!-- GENERADO por tools/agentsync desde %s. NO EDITAR A MANO. -->"

func renderClaude(a Agent) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", a.Name)
	fmt.Fprintf(&b, "description: %s\n", yamlString(a.Description))
	fmt.Fprintf(&b, "tools: %s\n", strings.Join(mapTools(a.Tools, runtimeClaude), ", "))
	b.WriteString("---\n\n")
	writeBody(&b, a)
	return []byte(b.String())
}

func renderCopilot(a Agent) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", a.Name)
	fmt.Fprintf(&b, "description: %s\n", yamlString(a.Description))
	tools := mapTools(a.Tools, runtimeCopilot)
	quoted := make([]string, len(tools))
	for i, t := range tools {
		quoted[i] = "'" + t + "'"
	}
	fmt.Fprintf(&b, "tools: [%s]\n", strings.Join(quoted, ", "))
	b.WriteString("---\n\n")
	writeBody(&b, a)
	return []byte(b.String())
}

func writeBody(b *strings.Builder, a Agent) {
	fmt.Fprintf(b, noticeFmt+"\n\n", a.Source)
	b.WriteString(a.Body)
	b.WriteString("\n")
	if len(a.Skills) > 0 {
		b.WriteString("\n## Procedimientos que debes cargar\n\n")
		for _, s := range a.Skills {
			fmt.Fprintf(b, "- @skills/%s.md\n", s)
		}
	}
}

const (
	runtimeClaude  = "claude"
	runtimeCopilot = "copilot"
)

func mapTools(canonical []string, runtime string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range canonical {
		m := toolMap[c]
		list := m.Claude
		if runtime == runtimeCopilot {
			list = m.Copilot
		}
		for _, t := range list {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

// yamlString colapsa la descripción plegada a una línea y la entrecomilla.
func yamlString(s string) string {
	return strconv.Quote(strings.Join(strings.Fields(s), " "))
}
