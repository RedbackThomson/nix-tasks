package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

func tasksNamed(names ...string) map[string]config.Task {
	tasks := map[string]config.Task{}
	for _, n := range names {
		tasks[n] = config.Task{}
	}
	return tasks
}

func TestLabels(t *testing.T) {
	tests := []struct {
		name  string
		tasks map[string]config.Task
		want  map[string]string
	}{
		{
			name: "design examples",
			tasks: tasksNamed(
				"generate-crossplane-provider-aws-ec2-managed-resources-crds",
				"generate-crossplane-provider-aws-ec2-managed-resources-mocks",
				"generate-crossplane-provider-aws-rds-managed-resources-crds",
				"test-unit-crossplane-provider-aws-ec2-managed-resources",
				"lint",
			),
			want: map[string]string{
				"generate-crossplane-provider-aws-ec2-managed-resources-crds":  "g-c-p-a-e-m-resources-crds",
				"generate-crossplane-provider-aws-ec2-managed-resources-mocks": "g-c-p-a-e-m-resources-mocks",
				"generate-crossplane-provider-aws-rds-managed-resources-crds":  "g-c-p-a-r-m-resources-crds",
				"test-unit-crossplane-provider-aws-ec2-managed-resources":      "t-u-c-p-a-e-managed-resources",
				"lint": "lint",
			},
		},
		{
			name:  "exactly 24 characters is kept",
			tasks: tasksNamed("abcdefghij-klmnopqrstuvw"),
			want:  map[string]string{"abcdefghij-klmnopqrstuvw": "abcdefghij-klmnopqrstuvw"},
		},
		{
			name:  "every separator is kept",
			tasks: tasksNamed("deploy.staging/us_east:service-api-gateway"),
			want:  map[string]string{"deploy.staging/us_east:service-api-gateway": "d.s/u_e:s-api-gateway"},
		},
		{
			name: "label wins",
			tasks: map[string]config.Task{
				"generate-crossplane-provider-aws-ec2-managed-resources-crds": {Label: "ec2-crds"},
				"build": {Label: "b"},
			},
			want: map[string]string{
				"generate-crossplane-provider-aws-ec2-managed-resources-crds": "ec2-crds",
				"build": "b",
			},
		},
		{
			name: "collisions keep more words until they differ",
			tasks: tasksNamed(
				"generate-crossplane-provider-aws-ec2-managed-resources-crds",
				"generate-crossplane-provider-azure-ec2-managed-resources-crds",
			),
			want: map[string]string{
				"generate-crossplane-provider-aws-ec2-managed-resources-crds":   "g-c-p-aws-ec2-managed-resources-crds",
				"generate-crossplane-provider-azure-ec2-managed-resources-crds": "g-c-p-azure-ec2-managed-resources-crds",
			},
		},
		{
			name: "only the colliding tasks grow",
			tasks: tasksNamed(
				"generate-crossplane-provider-aws-ec2-resources-crds",
				"generate-crossplane-provider-azure-ec2-resources-crds",
				"generate-crossplane-provider-gcp-compute-resources-crds",
			),
			want: map[string]string{
				"generate-crossplane-provider-aws-ec2-resources-crds":     "g-c-p-aws-ec2-resources-crds",
				"generate-crossplane-provider-azure-ec2-resources-crds":   "g-c-p-azure-ec2-resources-crds",
				"generate-crossplane-provider-gcp-compute-resources-crds": "g-c-p-g-c-resources-crds",
			},
		},
		{
			name: "an abbreviation clashing with a label grows",
			tasks: map[string]config.Task{
				"generate-crossplane-provider-aws-ec2-managed-resources-crds": {},
				"ec2": {Label: "g-c-p-a-e-m-resources-crds"},
			},
			want: map[string]string{
				"generate-crossplane-provider-aws-ec2-managed-resources-crds": "g-c-p-a-e-managed-resources-crds",
				"ec2": "g-c-p-a-e-m-resources-crds",
			},
		},
		{
			name:  "a long single word is never cut",
			tasks: tasksNamed("supercalifragilisticexpialidocious"),
			want:  map[string]string{"supercalifragilisticexpialidocious": "supercalifragilisticexpialidocious"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.Labels(tt.tasks); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Labels() =\n%v\nwant\n%v", got, tt.want)
			}
		})
	}
}

func TestLabels_IndependentOfRun(t *testing.T) {
	all := tasksNamed(
		"generate-crossplane-provider-aws-ec2-managed-resources-crds",
		"generate-crossplane-provider-azure-ec2-managed-resources-crds",
	)
	first := config.Labels(all)
	for range 20 {
		again := config.Labels(all)
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("labels changed between calls: %v vs %v", first, again)
		}
	}
}

func TestValidate_Labels(t *testing.T) {
	tests := []struct {
		name    string
		tasks   map[string]config.Task
		wantErr string
	}{
		{name: "unique labels", tasks: map[string]config.Task{"a": {Label: "x"}, "b": {Label: "y"}}},
		{name: "label equal to own name", tasks: map[string]config.Task{"a": {Label: "a"}}},
		{
			name:    "duplicate labels",
			tasks:   map[string]config.Task{"a": {Label: "x"}, "b": {Label: "x"}},
			wantErr: "task 'b': label 'x' is already used by task 'a'",
		},
		{
			name:    "label equal to another task's name",
			tasks:   map[string]config.Task{"a": {Label: "b"}, "b": {}},
			wantErr: "task 'a': label 'b' is the name of another task",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Validate(&config.Config{Tasks: tt.tasks})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestLabelWarnings(t *testing.T) {
	tasks := map[string]config.Task{
		"test-unit-crossplane-provider-aws-ec2-managed-resources":     {},
		"generate-crossplane-provider-aws-ec2-managed-resources-crds": {Label: "ec2-crds"},
		"lint": {},
	}
	got := config.LabelWarnings(tasks)
	want := []string{
		"task 'test-unit-crossplane-provider-aws-ec2-managed-resources': name is longer than 24 characters, so streamed output abbreviates it to 't-u-c-p-a-e-managed-resources'; set `label` to choose a shorter name",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LabelWarnings() = %q, want %q", got, want)
	}
}
