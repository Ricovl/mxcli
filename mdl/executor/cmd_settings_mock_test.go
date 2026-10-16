// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

func TestShowSettings_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{
				Model: &model.ModelSettings{
					HashAlgorithm: "BCrypt",
					JavaVersion:   "17",
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listSettings(ctx))

	out := buf.String()
	assertContainsStr(t, out, "Section")
	assertContainsStr(t, out, "Key Values")
}

func TestDescribeSettings_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{
				Model: &model.ModelSettings{
					HashAlgorithm: "BCrypt",
					JavaVersion:   "17",
					RoundingMode:  "HalfUp",
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeSettings(ctx, ""))
	assertContainsStr(t, buf.String(), "alter settings")
}

func TestShowSettings_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listSettings(ctx))
}

func TestDescribeSettings_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeSettings(ctx, ""))
}

func TestShowSettings_BackendError(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return nil, fmt.Errorf("connection lost")
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listSettings(ctx))
}

func TestShowSettings_JSON(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{
				Model: &model.ModelSettings{
					HashAlgorithm: "BCrypt",
					JavaVersion:   "17",
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON))
	assertNoError(t, listSettings(ctx))
	assertValidJSON(t, buf.String())
}

// mendixlabs/mxcli#1526: the project id (Business Events' app id) is shown,
// read-only — as a comment in describe so the output still executes, and as a
// row in show settings.
const testProjectID = "d4dec528-09bd-44f4-8daa-36658e546848"

func projectIDBackend() *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		GetProjectRootIDFunc: func() (string, error) { return testProjectID, nil },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{Model: &model.ModelSettings{HashAlgorithm: "BCrypt", JavaVersion: "17"}}, nil
		},
	}
}

func TestDescribeSettings_ShowsProjectIDAsComment(t *testing.T) {
	ctx, buf := newMockCtx(t, withBackend(projectIDBackend()))
	assertNoError(t, describeSettings(ctx, ""))
	out := buf.String()
	assertContainsStr(t, out, "-- Project ID: "+testProjectID)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, testProjectID) && !strings.HasPrefix(line, "--") {
			t.Errorf("the project id must only appear in a comment, got line %q", line)
		}
	}
}

func TestShowSettings_ShowsProjectID(t *testing.T) {
	ctx, buf := newMockCtx(t, withBackend(projectIDBackend()))
	assertNoError(t, listSettings(ctx))
	assertContainsStr(t, buf.String(), testProjectID)

	ctx, buf = newMockCtx(t, withBackend(projectIDBackend()), withFormat(FormatJSON))
	assertNoError(t, listSettings(ctx))
	assertValidJSON(t, buf.String())
	assertContainsStr(t, buf.String(), testProjectID)
}

// No project id available (a backend that cannot read it): nothing printed,
// nothing fails.
func TestSettings_NoProjectID(t *testing.T) {
	mb := projectIDBackend()
	mb.GetProjectRootIDFunc = func() (string, error) { return "", fmt.Errorf("unsupported") }
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeSettings(ctx, ""))
	if strings.Contains(buf.String(), "Project ID") {
		t.Errorf("no id, no line:\n%s", buf.String())
	}
	ctx, _ = newMockCtx(t, withBackend(mb))
	assertNoError(t, listSettings(ctx))
}
