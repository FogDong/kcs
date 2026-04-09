package cmd

import (
	"os"
	"testing"
)

func TestCheckConfig(t *testing.T) {
	const kubeDir = "/home/user/.kube"
	staticPath := kubeDir + "/kcs-config"

	// Override XDG_RUNTIME_DIR so SessionPath() returns a predictable value.
	os.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	os.Setenv("KCS_SESSION", "testsession")
	defer os.Unsetenv("XDG_RUNTIME_DIR")
	defer os.Unsetenv("KCS_SESSION")

	sessionPath := "/run/user/1000/kcs/sessions/testsession"

	cases := []struct {
		name       string
		kubeconfig string
		want       configStatus
	}{
		{
			name: "unset KUBECONFIG",
			want: configUnset,
		},
		{
			name:       "both session and static paths",
			kubeconfig: sessionPath + ":" + staticPath,
			want:       configOK,
		},
		{
			name:       "both paths with extras",
			kubeconfig: sessionPath + ":" + staticPath + ":/home/user/.kube/config",
			want:       configOK,
		},
		{
			name:       "static only (missing session)",
			kubeconfig: staticPath,
			want:       configMissingSession,
		},
		{
			name:       "session only (missing static)",
			kubeconfig: sessionPath,
			want:       configNotKCS,
		},
		{
			name:       "unrelated KUBECONFIG",
			kubeconfig: "/home/user/.kube/config",
			want:       configNotKCS,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("KUBECONFIG", tc.kubeconfig)
			defer os.Unsetenv("KUBECONFIG")

			got := checkConfig(kubeDir)
			if got != tc.want {
				t.Errorf("checkConfig() = %v, want %v", got, tc.want)
			}
		})
	}
}
