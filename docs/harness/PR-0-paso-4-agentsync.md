# PR-0 · Paso 4 — `tools/agentsync`

**Continúa desde:** `PR-0.md`, pasos 1 a 3 completados.
**Ubicación recomendada:** `docs/harness/PR-0-paso-4.md`.

> Todo el código de este documento está compilado y probado. Los tests se
> incluyen y pasan.

---

## Índice

1. [Qué problema resuelve](#1-qué-problema-resuelve)
2. [Decisiones de diseño del generador](#2-decisiones-de-diseño-del-generador)
3. [Crear el módulo](#3-crear-el-módulo)
4. [`canonical.go` — parseo y presupuesto](#4-canonicalgo--parseo-y-presupuesto)
5. [`render.go` — los dos adaptadores](#5-rendergo--los-dos-adaptadores)
6. [`main.go` — modos generate y check](#6-maingo--modos-generate-y-check)
7. [`agentsync_test.go` — los tests](#7-agentsync_testgo--los-tests)
8. [Ejecutar y verificar](#8-ejecutar-y-verificar)
9. [Commit](#9-commit)
10. [Qué falta](#10-qué-falta)

---

## 1. Qué problema resuelve

Tienes cinco definiciones canónicas en `agents/`. Ningún runtime las lee: Claude
Code espera `.claude/agents/*.md` y Copilot espera `.github/agents/*.agent.md`,
cada uno con su propio vocabulario de herramientas.

La solución ingenua es mantener las tres copias a mano. Falla siempre, y falla
de la peor manera posible: **en silencio**. Alguien ajusta una instrucción en la
versión de Claude, olvida la de Copilot, y a partir de ese momento tu comparación
entre runtimes mide dos prompts distintos en lugar de dos runtimes. El
experimento queda invalidado sin que ningún check se ponga en rojo.

`agentsync` convierte eso en imposible. Una fuente, dos salidas generadas, y un
check de CI que falla si lo generado no coincide con lo que produciría el
generador ahora mismo.

Hace tres cosas:

| Función | Modo generate | Modo check |
|---|---|---|
| Traducir definiciones a los dos formatos | Escribe los ficheros | Compara y reporta diferencias |
| Verificar el presupuesto de instrucciones | Reporta excesos | Reporta excesos |
| Detectar adaptadores huérfanos | Los elimina | Los reporta |

Los huérfanos merecen una nota. Si renombras `backend-builder` a
`go-builder`, el adaptador antiguo se queda en disco, y **el runtime lo seguirá
cargando**: tendrías dos agentes activos, uno de ellos con instrucciones
obsoletas y sin fuente que las gobierne. Es un fallo difícil de diagnosticar
porque nada está roto — simplemente hay un agente fantasma.

---

## 2. Decisiones de diseño del generador

### 2.1 Módulo Go propio

`tools/agentsync` es su propio módulo, no parte de `backend/`.

Misma lógica que aplicamos al acotar el módulo del backend: son ecosistemas con
ciclos de vida distintos. El backend se despliega; esto es utillaje del
repositorio. Si compartieran módulo, una dependencia del generador acabaría en
el `go.sum` del servicio que va a producción, y `govulncheck` reportaría sobre
código que nunca se despliega.

### 2.2 Cero dependencias externas

El generador usa solo la librería estándar, incluido el parseo del frontmatter.

Podría haber usado `gopkg.in/yaml.v3`. No lo hace por dos razones. La primera es
que **una herramienta que corre en cada PR no debería tener superficie de
suministro propia**: es el gate que valida el arnés, y un gate con dependencias
transitivas es un gate que puede romperse por algo ajeno al repositorio. La
segunda es que el subconjunto de YAML que usamos es minúsculo —escalares,
listas en línea y continuaciones indentadas— y un parser de 30 líneas lo cubre
con mensajes de error mucho mejores que los de un parser genérico.

El coste: si algún día necesitas YAML anidado en el frontmatter, hay que ampliar
el parser. Es un coste conocido y acotado.

### 2.3 El aviso de "no editar" va en el cuerpo, no antes del frontmatter

Ambos runtimes esperan el frontmatter en el byte cero del fichero. Un comentario
HTML antes del `---` rompería el parseo. Por eso el aviso es la primera línea
**después** del bloque de frontmatter.

La protección real contra ediciones manuales es doble: `.gitattributes` los
colapsa en los diffs, y el modo `-check` falla el PR.

### 2.4 La traducción de herramientas es un mapa, no lógica dispersa

Todo el conocimiento sobre cómo llama cada runtime a sus herramientas vive en
una única variable, `toolMap`. Si mañana un runtime renombra `runCommands`, se
toca una línea.

> **Verifica los identificadores.** Los valores del mapa son los que uso como
> punto de partida; los nombres de herramientas de cada runtime cambian entre
> versiones. Contrasta con la documentación vigente de Claude Code y de Copilot
> y ajusta el mapa. El mecanismo es lo que importa: la lista concreta es un dato
> y está aislada precisamente para que corregirla sea trivial.

---

## 3. Crear el módulo

```bash
cd tools/agentsync
cat > go.mod <<'EOF'
module github.com/fexlixjhl/task-allocation/tools/agentsync

go 1.25
EOF
cd ../..
```

No hace falta `go.work`: los dos módulos son independientes y no se importan
entre sí.

---

## 4. `canonical.go` — parseo y presupuesto

Fichero `tools/agentsync/canonical.go`:

```go
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
```

### Lo que merece atención

**La validación de `load` es deliberadamente estricta.** Cinco condiciones que
abortan: nombre ausente, nombre que no coincide con el fichero, descripción
ausente, `tools` vacío y clase de permiso inválida. Un agente mal definido debe
fallar al generar, no comportarse de forma rara en producción tres días después.

**`a.Name != base` parece pedantería y no lo es.** Si el fichero se llama
`code-reviewer.md` pero el frontmatter dice `name: security-reviewer`, tienes dos
agentes con la misma identidad y el runtime cargará uno de forma impredecible.
Forzar la coincidencia hace que el nombre de fichero sea la identidad, que es lo
que asumen los humanos al leer el árbol.

**La validación contra `toolMap` cierra el círculo del mapa de herramientas.** Un
`tools: [network]` inventado no se ignora en silencio: falla con un mensaje que
dice exactamente qué hacer. Es la diferencia entre un agente que
misteriosamente no puede hacer algo y un error accionable.

**`contentLines` es la métrica del presupuesto.** Ignora líneas vacías y
encabezados porque no consumen atención del modelo de forma comparable a una
directiva. No es una medida perfecta —nada lo es— pero es estable, determinista
y suficiente para detectar la deriva, que es lo que queremos: no la cifra exacta,
sino la señal de que un fichero de instrucciones está creciendo sin control.

---

## 5. `render.go` — los dos adaptadores

Fichero `tools/agentsync/render.go`:

```go
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
```

### Lo que merece atención

**`edit` implica `Read`, `Edit` y `Write` en Claude.** Un agente que edita
necesita leer primero; la mayoría de runtimes exigen haber leído un fichero antes
de modificarlo. Expresarlo en el mapa evita que cada definición tenga que
declarar `[read, edit]` de forma redundante — y, más importante, evita el error
inverso: que alguien declare `[edit]` a secas y el agente falle al primer intento
por no poder leer.

**La deduplicación de `mapTools` importa por lo mismo.** `[read, edit]` expande a
`Read, Read, Edit, Write`; sin dedup, generarías frontmatter con duplicados que
algunos parsers rechazan.

**El orden de salida es determinista** porque respeta el orden de declaración y
el `seen` preserva la primera aparición. Sin determinismo, cada ejecución
produciría un diff distinto y el modo `-check` sería inservible.

**Los skills se inyectan como referencias `@skills/*.md` al final del cuerpo.**
Así el agente carga el procedimiento bajo demanda en lugar de llevarlo embebido,
que es exactamente la capa 3 de la arquitectura de instrucciones: presupuesto
gastado solo cuando se necesita.

**`yamlString` usa `strconv.Quote`.** La descripción canónica está plegada en
varias líneas; en el frontmatter generado debe ser una sola línea entrecomillada.
`strconv.Quote` escapa comillas y barras, que es lo que puede romper el YAML.

---

## 6. `main.go` — modos generate y check

Fichero `tools/agentsync/main.go`:

```go
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
```

### Lo que merece atención

**Tres códigos de salida distintos, y no es cosmético.** `0` correcto, `1`
el arnés está desactualizado o fuera de presupuesto, `2` error del propio
generador. En el workflow de CI la distinción importa: un `1` significa "regenera
y commitea", un `2` significa "el generador está roto y hay que arreglarlo". Un
único código de error obligaría a leer logs para saber qué hacer.

**`run` devuelve los problemas en lugar de imprimirlos.** Es lo que permite
testear la lógica sin capturar stdout, y por eso los tests del apartado 7 pueden
verificar el comportamiento completo de extremo a extremo.

**Se comparan bytes, no semántica.** `bytes.Equal` es intencionadamente
inflexible: cualquier edición manual del generado, por inocua que sea, se
detecta. La alternativa —comparar el contenido ignorando espacios— dejaría hueco
para modificaciones "pequeñas" que erosionan la fuente única.

**Los huérfanos se borran al generar y se reportan al comprobar.** Asimetría
deliberada: en local quieres que se limpie solo; en CI quieres saberlo, porque
significa que alguien regeneró mal o commiteó a medias.

**El mensaje final `ejecuta make generate y commitea el resultado`.** El error de
un check debe decir qué hacer, no solo qué está mal. Es la diferencia entre un
gate que enseña y uno que frustra — y, en un flujo agéntico, entre un agente que
se autocorrige y uno que se atasca.

---

## 7. `agentsync_test.go` — los tests

Fichero `tools/agentsync/agentsync_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `---
name: demo-builder
description: Hace algo concreto y muy útil
  en más de una línea plegada.
tools: [read, edit, bash]
skills: [github-pr-protocol]
permission-class: builder
---

Cuerpo del agente.

## Qué no haces
- Nada raro.
`

func writeSample(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, canonicalDir), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, canonicalDir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadParsesFoldedDescription(t *testing.T) {
	dir := t.TempDir()
	writeSample(t, dir, "demo-builder.md", sample)

	a, err := load(filepath.Join(dir, canonicalDir, "demo-builder.md"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := "Hace algo concreto y muy útil en más de una línea plegada."
	if a.Description != want {
		t.Errorf("description = %q, quiero %q", a.Description, want)
	}
	if len(a.Tools) != 3 || a.Tools[2] != "bash" {
		t.Errorf("tools = %v", a.Tools)
	}
	if a.PermissionClass != "builder" {
		t.Errorf("permission-class = %q", a.PermissionClass)
	}
}

func TestLoadRejectsNameMismatch(t *testing.T) {
	dir := t.TempDir()
	writeSample(t, dir, "otro-nombre.md", sample)
	if _, err := load(filepath.Join(dir, canonicalDir, "otro-nombre.md")); err == nil {
		t.Fatal("se esperaba error por desajuste entre name y fichero")
	}
}

func TestLoadRejectsUnknownTool(t *testing.T) {
	dir := t.TempDir()
	bad := strings.Replace(sample, "tools: [read, edit, bash]", "tools: [read, teletransporte]", 1)
	writeSample(t, dir, "demo-builder.md", bad)
	if _, err := load(filepath.Join(dir, canonicalDir, "demo-builder.md")); err == nil {
		t.Fatal("se esperaba error por herramienta desconocida")
	}
}

func TestMapToolsDeduplicates(t *testing.T) {
	got := mapTools([]string{"read", "edit"}, runtimeClaude)
	if strings.Join(got, ",") != "Read,Edit,Write" {
		t.Errorf("mapTools = %v, quiero [Read Edit Write]", got)
	}
}

func TestReviewerHasNoWriteCapability(t *testing.T) {
	for _, rt := range []string{runtimeClaude, runtimeCopilot} {
		for _, tool := range mapTools([]string{"read", "search"}, rt) {
			switch tool {
			case "Edit", "Write", "Bash", "edit", "runCommands":
				t.Errorf("runtime %s: un reviewer no debe recibir %q", rt, tool)
			}
		}
	}
}

func TestContentLinesIgnoresHeadingsAndBlanks(t *testing.T) {
	if n := contentLines("# Título\n\nuna\n\n## Otro\ndos\n"); n != 2 {
		t.Errorf("contentLines = %d, quiero 2", n)
	}
}

func TestRunGeneratesThenChecksClean(t *testing.T) {
	dir := t.TempDir()
	writeSample(t, dir, "demo-builder.md", sample)

	if problems, err := run(dir, false); err != nil || len(problems) > 0 {
		t.Fatalf("generate: err=%v problems=%v", err, problems)
	}
	if problems, err := run(dir, true); err != nil || len(problems) > 0 {
		t.Fatalf("check tras generate debería estar limpio: err=%v problems=%v", err, problems)
	}

	// Un adaptador editado a mano debe detectarse.
	p := filepath.Join(dir, claudeDir, "demo-builder.md")
	if err := os.WriteFile(p, []byte("editado a mano\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	problems, err := run(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) == 0 {
		t.Fatal("se esperaba detectar el adaptador desactualizado")
	}
}

func TestRunDetectsOrphans(t *testing.T) {
	dir := t.TempDir()
	writeSample(t, dir, "demo-builder.md", sample)
	if _, err := run(dir, false); err != nil {
		t.Fatal(err)
	}

	// Simula un agente renombrado: queda un adaptador sin origen.
	stale := filepath.Join(dir, claudeDir, "agente-borrado.md")
	if err := os.WriteFile(stale, []byte("obsoleto\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	problems, err := run(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) == 0 {
		t.Fatal("se esperaba detectar el huérfano")
	}
	if _, err := run(dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("el huérfano debería haberse eliminado al regenerar")
	}
}
```

### El test que más te va a servir

`TestReviewerHasNoWriteCapability` no prueba código: **prueba una política de
seguridad.**

Verifica que la combinación de capacidades de un revisor —`read` y `search`—
jamás expande a una herramienta de escritura en ningún runtime. Si dentro de seis
meses alguien añade `"search": {Claude: [..., "Write"]}` al mapa por descuido,
este test se pone en rojo y el PR no pasa.

Es la separación de funciones convertida en test unitario. Merece la pena
interiorizar el patrón: **cuando un control de seguridad depende de una tabla de
configuración, esa tabla es testeable, y ese test vale más que cualquier
comentario.** [SDL: gobierno · OWASP: ASVS V1]

---

## 8. Ejecutar y verificar

```bash
# tests del generador (desde su propio módulo)
go -C tools/agentsync vet ./...
go -C tools/agentsync test ./...

# generar
go -C tools/agentsync run . -root ../..
```

> **Por qué `go -C` y no `go run ./tools/agentsync`.** No hay módulo Go en la
> raíz del repositorio — el del backend cuelga de `backend/` y el del generador
> de `tools/agentsync/`, tal y como decidimos en el paso 1.4 y en el 2.1.
> Invocar `go run ./tools/agentsync` desde la raíz falla con
> `cannot find main module`, porque Go busca un `go.mod` en el directorio actual
> y sus padres, no en el subdirectorio que le pasas.
>
> `go -C <dir>` cambia de directorio antes de ejecutar el comando, así que el
> módulo se resuelve correctamente y tú no cambias de sitio. El `-root ../..`
> es relativo al nuevo directorio de trabajo, y apunta de vuelta a la raíz del
> repositorio.
>
> Equivalente con subshell, si prefieres: `(cd tools/agentsync && go run . -root ../..)`.

Salida esperada:

```
agentsync: adaptadores regenerados
```

Inspecciona un adaptador generado:

```bash
head -8 .claude/agents/backend-builder.md
```

```
---
name: backend-builder
description: "Implementa backend en Go a partir de un PLAN.md aprobado y del contrato OpenAPI. Úsalo solo en fase Implement, nunca antes."
tools: Read, Edit, Write, Bash, Grep, Glob
---

<!-- GENERADO por tools/agentsync desde agents/backend-builder.md. NO EDITAR A MANO. -->
```

Y su equivalente de Copilot:

```bash
head -5 .github/agents/backend-builder.agent.md
tail -6 .github/agents/backend-builder.agent.md
```

```
---
name: backend-builder
description: "Implementa backend en Go a partir de un PLAN.md aprobado y del contrato OpenAPI. Úsalo solo en fase Implement, nunca antes."
tools: ['search', 'edit', 'runCommands']
---

## Procedimientos que debes cargar

- @skills/openapi-contract-reading.md
- @skills/hexagonal-boundaries.md
- @skills/github-pr-protocol.md
```

**Fíjate en la diferencia entre las dos líneas `tools:`.** Es exactamente el
trabajo que el generador te ahorra mantener a mano, y donde se produciría la
divergencia silenciosa que invalidaría la comparación entre runtimes.

Ahora comprueba el modo `-check`:

```bash
go -C tools/agentsync run . -root ../.. -check
# agentsync: arnés al día

# Provoca un fallo a propósito para verlo funcionar
echo "editado a mano" >> .claude/agents/backend-builder.md
go -C tools/agentsync run . -root ../.. -check; echo "código de salida: $?"
```

```
agentsync: el arnés no está al día:
  - .claude/agents/backend-builder.md: desactualizado
  - ejecuta `make generate` y commitea el resultado
código de salida: 1
```

Restaura:

```bash
go -C tools/agentsync run . -root ../..
go -C tools/agentsync run . -root ../.. -check
```

Comprueba también el presupuesto de la capa de hechos:

```bash
grep -v '^\s*$' AGENTS.md | grep -vc '^\s*#'
```

Debe dar **30**, por debajo del límite de 40. El margen es estrecho a propósito:
cuando quieras añadir algo a `AGENTS.md`, tendrás que preguntarte si de verdad
no lo puede verificar una herramienta.

---

## 9. Commit

Los adaptadores generados **sí se commitean**. Podría parecer contradictorio
—son artefactos derivados— pero es necesario: los runtimes los leen directamente
del checkout, sin paso de build previo. Por eso existe el modo `-check`, que
sustituye a la garantía que daría no versionarlos.

```bash
git add tools/agentsync .claude .github/agents
git commit -m "chore(agents): generador de adaptadores para ambos runtimes

agentsync produce .claude/agents/ y .github/agents/ desde las definiciones
canónicas de agents/, verifica el presupuesto de instrucciones y detecta
adaptadores huérfanos. Sin dependencias externas.

Agent: none
Runtime: human"
```

Verificación final del paso:

```bash
ls .claude/agents/          # 5 ficheros .md
ls .github/agents/          # 5 ficheros .agent.md
git check-attr linguist-generated -- .claude/agents/backend-builder.md
# .claude/agents/backend-builder.md: linguist-generated: set
```

Checklist:

- [ ] `tools/agentsync/go.mod` con módulo propio y `go 1.25`
- [ ] Cuatro ficheros `.go` creados
- [ ] `go -C tools/agentsync vet ./...` limpio
- [ ] `go -C tools/agentsync test ./...` en verde
- [ ] Cinco adaptadores en `.claude/agents/`
- [ ] Cinco adaptadores en `.github/agents/`
- [ ] `-check` sale limpio y con código 0
- [ ] `-check` sale con código 1 si editas un generado a mano
- [ ] `AGENTS.md` por debajo de 40 líneas de contenido
- [ ] Generados marcados como `linguist-generated`

Sigue sin haber push: PR-0 se abre completo al terminar el paso 6.

---

## 10. Qué falta

| Paso | Contenido |
|---|---|
| **5** | Los cinco skills: `openapi-contract-reading`, `github-pr-protocol`, `hexagonal-boundaries`, `rpi-artifacts`, `owasp-asvs-review` |
| **6** | `Makefile`, devcontainer, workflow `harness-check`, plantilla de PR, `CODEOWNERS`, `docs/security/exceptions.md`, protección de rama, y apertura del PR |

Nota para el paso 6: el workflow `harness-check` se reduce prácticamente a
`go -C tools/agentsync run . -root ../.. -check`. Toda la lógica del gate ya
está escrita y
testeada aquí — el workflow solo la invoca. Es el patrón que buscamos en todo el
proyecto: la verificación vive en una herramienta con tests propios, no en YAML
de CI.
