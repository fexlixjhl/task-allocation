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
