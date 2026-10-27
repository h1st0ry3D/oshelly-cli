//go:build linux

package launch

import (
	"errors"
	"strings"
	"testing"
)

func TestTheOmarchyConfigEditorIsPreferred(t *testing.T) {
	e := testEnv(nil, map[string]string{
		"omarchy-launch-config-editor": "/usr/share/omarchy/bin/omarchy-launch-config-editor",
		"omarchy-launch-editor":        "/usr/share/omarchy/bin/omarchy-launch-editor",
		"xdg-open":                     "/usr/bin/xdg-open",
	})

	plan, err := editorPlan("/home/tester/.config/oshelly/oshelly.conf", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "omarchy-launch-config-editor" {
		t.Errorf("chose %q, want the Omarchy config editor", plan.How)
	}
	if len(plan.Args) != 1 || plan.Args[0] != "/home/tester/.config/oshelly/oshelly.conf" {
		t.Errorf("args = %#v, want only the path", plan.Args)
	}
}

func TestTheEditorFallsBackToTheOmarchyEditorThenXDGOpen(t *testing.T) {
	e := testEnv(nil, map[string]string{
		"omarchy-launch-editor": "/usr/share/omarchy/bin/omarchy-launch-editor",
		"xdg-open":              "/usr/bin/xdg-open",
	})
	plan, err := editorPlan("/tmp/oshelly.conf", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "omarchy-launch-editor" {
		t.Errorf("chose %q, want omarchy-launch-editor", plan.How)
	}

	e = testEnv(nil, map[string]string{"xdg-open": "/usr/bin/xdg-open"})
	plan, err = editorPlan("/tmp/oshelly.conf", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "xdg-open" {
		t.Errorf("chose %q, want xdg-open", plan.How)
	}
}

func TestOSHELLYEditorOverridesEverything(t *testing.T) {
	e := testEnv(
		map[string]string{"OSHELLY_EDITOR": "/opt/my-editor"},
		map[string]string{
			"/opt/my-editor":               "/opt/my-editor",
			"omarchy-launch-config-editor": "/usr/share/omarchy/bin/omarchy-launch-config-editor",
		})

	plan, err := editorPlan("/tmp/oshelly.conf", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "OSHELLY_EDITOR" {
		t.Errorf("chose %q, want the override", plan.How)
	}
}

func TestAMissingEditorOverrideIsAnError(t *testing.T) {
	e := testEnv(
		map[string]string{"OSHELLY_EDITOR": "/opt/not-there"},
		map[string]string{"xdg-open": "/usr/bin/xdg-open"})

	_, err := editorPlan("/tmp/oshelly.conf", e)
	if err == nil {
		t.Fatal("a missing editor override was ignored")
	}
	if !strings.Contains(err.Error(), "/opt/not-there") {
		t.Errorf("error %q does not name the value", err)
	}
}

func TestNoEditorIsReportedAsSuch(t *testing.T) {
	_, err := editorPlan("/tmp/oshelly.conf", testEnv(nil, nil))
	if !errors.Is(err, ErrNoEditor) {
		t.Errorf("err = %v, want ErrNoEditor", err)
	}
}
