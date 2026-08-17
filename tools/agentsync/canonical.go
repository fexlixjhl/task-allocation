package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Presupuestos de la arquitectura de instrucciones en tres capas.
const (
	maxAgentInstructions = 40
	maxFactsLines        = 40
	maxSkillLines        = 80
)

// Agent es una definición canónica de agents/<name>.md.
type Agent struct {
	Name            string
	Description     string
	Tools           []string
	Skills          []string
	PermissionClass string

	Body   string
	Source string
}

var validClasses = map[string]bool{
	"designer":   true,
	"researcher": true,
	"planner":    true,
	"builder":    true,
	"reviewer":   true,
	"gate":       true,
}

func loadAll(dir string) ([]Agent, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("no se puede leer %s: %w", dir, err)
	}
	var agents []Agent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		a, err := load(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents, nil
}

func load(path string) (Agent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Agent{}, err
	}
	front, body, err := splitFrontmatter(string(raw))
	if err != nil {
		return Agent{}, fmt.Errorf("%s: %w", path, err)
	}
	fields, err := parseFields(front)
	if err != nil {
		return Agent{}, fmt.Errorf("%s: %w", path, err)
	}

	a := Agent{
		Name:            fields["name"],
		Description:     fields["description"],
		Tools:           parseList(fields["tools"]),
		Skills:          parseList(fields["skills"]),
		PermissionClass: fields["permission-class"],
		Body:            strings.TrimSpace(body),
		Source:          filepath.ToSlash(path),
	}

	base := strings.TrimSuffix(filepath.Base(path), ".md")
	switch {
	case a.Name == "":
		return Agent{}, fmt.Errorf("%s: falta 'name'", path)
	case a.Name != base:
		return Agent{}, fmt.Errorf("%s: 'name' (%q) no coincide con el nombre de fichero", path, a.Name)
	case a.Description == "":
		return Agent{}, fmt.Errorf("%s: falta 'description'", path)
	case len(a.Tools) == 0:
		return Agent{}, fmt.Errorf("%s: falta 'tools'", path)
	case !validClasses[a.PermissionClass]:
		return Agent{}, fmt.Errorf("%s: 'permission-class' inválida: %q", path, a.PermissionClass)
	}
	for _, t := range a.Tools {
		if _, ok := toolMap[t]; !ok {
			return Agent{}, fmt.Errorf("%s: herramienta desconocida %q; añádela a toolMap", path, t)
		}
	}
	return a, nil
}

// splitFrontmatter separa el bloque YAML inicial del cuerpo markdown.
func splitFrontmatter(s string) (front, body string, err error) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	const open = "---\n"
	if !strings.HasPrefix(s, open) {
		return "", "", fmt.Errorf("no empieza con frontmatter '---'")
	}
	rest := s[len(open):]
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		return "", "", fmt.Errorf("frontmatter sin cierre '---'")
	}
	return rest[:i], rest[i+len("\n---\n"):], nil
}

// parseFields entiende el subconjunto de YAML que usamos: escalares,
// listas en línea y continuaciones plegadas indentadas.
func parseFields(front string) (map[string]string, error) {
	fields := map[string]string{}
	last := ""
	for n, line := range strings.Split(front, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if last == "" {
				return nil, fmt.Errorf("línea %d: continuación sin clave previa", n+1)
			}
			fields[last] += " " + strings.TrimSpace(line)
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("línea %d: se esperaba 'clave: valor'", n+1)
		}
		key = strings.TrimSpace(key)
		fields[key] = strings.TrimSpace(value)
		last = key
	}
	return fields, nil
}

func parseList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// contentLines cuenta líneas que gastan presupuesto de instrucciones:
// ni vacías, ni encabezados, ni frontmatter.
func contentLines(s string) int {
	if _, body, err := splitFrontmatter(s); err == nil {
		s = body
	}
	n := 0
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		n++
	}
	return n
}
