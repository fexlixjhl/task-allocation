// agentsync genera los adaptadores de agente de cada runtime a partir de las
// definiciones canónicas de agents/, y verifica el presupuesto de instrucciones.
//
//	agentsync            genera y escribe
//	agentsync -check     no escribe; falla si algo está desactualizado
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	canonicalDir = "agents"
	skillsDir    = "skills"
	claudeDir    = ".claude/agents"
	copilotDir   = ".github/agents"
	factsFile    = "AGENTS.md"
)

func main() {
	check := flag.Bool("check", false, "verifica sin escribir; sale con código 1 si algo está desactualizado")
	root := flag.String("root", ".", "raíz del repositorio")
	flag.Parse()

	problems, err := run(*root, *check)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentsync: "+err.Error())
		os.Exit(2)
	}
	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "agentsync: el arnés no está al día:")
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  - "+p)
		}
		os.Exit(1)
	}
	if *check {
		fmt.Println("agentsync: arnés al día")
	} else {
		fmt.Println("agentsync: adaptadores regenerados")
	}
}

func run(root string, check bool) ([]string, error) {
	agents, err := loadAll(filepath.Join(root, canonicalDir))
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("no hay definiciones en %s/", canonicalDir)
	}

	var problems []string

	// Presupuesto de la capa de rol.
	for _, a := range agents {
		if n := contentLines(a.Body); n > maxAgentInstructions {
			problems = append(problems, fmt.Sprintf("%s: %d instrucciones, máximo %d", a.Source, n, maxAgentInstructions))
		}
	}

	// Presupuesto de la capa de hechos.
	if raw, err := os.ReadFile(filepath.Join(root, factsFile)); err == nil {
		if n := contentLines(string(raw)); n > maxFactsLines {
			problems = append(problems, fmt.Sprintf("%s: %d líneas de contenido, máximo %d", factsFile, n, maxFactsLines))
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Skills referenciados: deben existir y caber en presupuesto.
	skillProblems, err := checkSkills(root, agents)
	if err != nil {
		return nil, err
	}
	problems = append(problems, skillProblems...)
	// Artefactos esperados.
	want := map[string][]byte{}
	for _, a := range agents {
		want[filepath.Join(root, claudeDir, a.Name+".md")] = renderClaude(a)
		want[filepath.Join(root, copilotDir, a.Name+".agent.md")] = renderCopilot(a)
	}

	paths := make([]string, 0, len(want))
	for p := range want {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		if check {
			got, err := os.ReadFile(p)
			if err != nil {
				problems = append(problems, rel(root, p)+": falta")
				continue
			}
			if !bytes.Equal(got, want[p]) {
				problems = append(problems, rel(root, p)+": desactualizado")
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, want[p], 0o644); err != nil {
			return nil, err
		}
	}

	// Huérfanos: adaptadores de agentes que ya no existen en agents/.
	orphans, err := findOrphans(root, want)
	if err != nil {
		return nil, err
	}
	for _, o := range orphans {
		if check {
			problems = append(problems, rel(root, o)+": huérfano, ya no existe su definición canónica")
			continue
		}
		if err := os.Remove(o); err != nil {
			return nil, err
		}
	}

	if len(problems) > 0 && check {
		problems = append(problems, "ejecuta `make generate` y commitea el resultado")
	}
	return problems, nil
}

// checkSkills verifica que cada skill referenciado por un agente existe en
// skills/ y respeta el presupuesto de la capa de procedimiento.
func checkSkills(root string, agents []Agent) ([]string, error) {
	referencedBy := map[string][]string{}
	for _, a := range agents {
		for _, s := range a.Skills {
			referencedBy[s] = append(referencedBy[s], a.Name)
		}
	}
	names := make([]string, 0, len(referencedBy))
	for s := range referencedBy {
		names = append(names, s)
	}
	sort.Strings(names)

	var problems []string
	for _, s := range names {
		p := filepath.Join(root, skillsDir, s+".md")
		raw, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			problems = append(problems, fmt.Sprintf(
				"%s/%s.md: no existe; lo referencian: %s",
				skillsDir, s, strings.Join(referencedBy[s], ", ")))
			continue
		}
		if err != nil {
			return nil, err
		}
		if n := contentLines(string(raw)); n > maxSkillLines {
			problems = append(problems, fmt.Sprintf(
				"%s/%s.md: %d líneas de contenido, máximo %d", skillsDir, s, n, maxSkillLines))
		}
	}
	return problems, nil
}

func findOrphans(root string, want map[string][]byte) ([]string, error) {
	var orphans []string
	for _, dir := range []string{claudeDir, copilotDir} {
		full := filepath.Join(root, dir)
		entries, err := os.ReadDir(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			p := filepath.Join(full, e.Name())
			if _, ok := want[p]; !ok {
				orphans = append(orphans, p)
			}
		}
	}
	sort.Strings(orphans)
	return orphans, nil
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}
