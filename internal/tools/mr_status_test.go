package tools

import (
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestMergeStatusAdviceCoversKnownValues(t *testing.T) {
	keys := []string{
		"mergeable", "checking", "unchecked", "preparing", "approvals_syncing",
		"ci_still_running", "ci_must_pass", "commits_status", "conflict", "need_rebase",
		"discussions_not_resolved", "draft_status", "not_approved", "requested_changes",
		"not_open", "merge_request_blocked", "merge_time", "status_checks_must_pass",
		"security_policy_pipeline_check", "security_policy_violations",
		"jira_association_missing", "title_regex", "locked_paths", "locked_lfs_files",
	}
	for _, k := range keys {
		if strings.TrimSpace(mergeStatusAdvice[k]) == "" {
			t.Errorf("no advice for %q", k)
		}
		mr := &gitlab.MergeRequest{}
		mr.DetailedMergeStatus = k
		got := statusAdvice(mr)
		if !strings.HasPrefix(got, "detailed_merge_status: "+k+" — ") {
			t.Errorf("statusAdvice(%q) = %q", k, got)
		}
		if strings.Contains(got, unknownStatusAdvice) {
			t.Errorf("known status %q got the generic hint: %q", k, got)
		}
	}
}

func TestStatusAdviceContent(t *testing.T) {
	tests := []struct {
		status   string
		contains []string
	}{
		{"checking", []string{"проверка ещё идёт", "повторите get_merge_request через несколько секунд"}},
		{"unchecked", []string{"проверка ещё идёт", "повторите get_merge_request через несколько секунд"}},
		{"draft_status", []string{"update_merge_request", "Draft:"}},
		{"not_approved", []string{"MRX-02"}},
		{"ci_still_running", []string{"auto-merge не поддерживается"}},
		{"brand_new_value", []string{"detailed_merge_status: brand_new_value — ", unknownStatusAdvice}},
		{"", []string{"статус слияния не вернулся"}},
	}
	for _, tt := range tests {
		mr := &gitlab.MergeRequest{}
		mr.DetailedMergeStatus = tt.status
		got := statusAdvice(mr)
		for _, want := range tt.contains {
			if !strings.Contains(got, want) {
				t.Errorf("statusAdvice(%q) = %q, missing %q", tt.status, got, want)
			}
		}
	}
}

func TestStateAdvice(t *testing.T) {
	tests := []struct {
		state    string
		wantOK   bool
		contains string
	}{
		{"opened", false, ""},
		{"merged", true, "уже влит"},
		{"closed", true, "state_event=reopen"},
		{"locked", true, "заблокирован"},
		{"weird", true, "weird"},
	}
	for _, tt := range tests {
		mr := &gitlab.MergeRequest{}
		mr.State = tt.state
		text, ok := stateAdvice(mr)
		if ok != tt.wantOK {
			t.Errorf("stateAdvice(%q) ok = %v, want %v", tt.state, ok, tt.wantOK)
		}
		if !strings.Contains(text, tt.contains) {
			t.Errorf("stateAdvice(%q) = %q, missing %q", tt.state, text, tt.contains)
		}
	}
}
