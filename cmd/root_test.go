package cmd

import (
	"os"
	"testing"
)

func TestCheckConfig(t *testing.T) {
	const kubeDir = "/home/user/.kube"
	staticPath := kubeDir + "/kcs-config"

	cases := []struct {
		name       string
		kubeconfig string
		kcsSession string
		want       configStatus
	}{
		{
			name: "unset KUBECONFIG",
			want: configUnset,
		},
		{
			name:       "static path only",
			kubeconfig: staticPath,
			want:       configOK,
		},
		{
			name:       "static path in multi-path KUBECONFIG",
			kubeconfig: staticPath + ":/home/user/.kube/config",
			want:       configOK,
		},
		{
			name:       "static path in middle of KUBECONFIG",
			kubeconfig: "/home/user/.kube/config:" + staticPath + ":/other/config",
			want:       configOK,
		},
		{
			name:       "unrelated KUBECONFIG",
			kubeconfig: "/home/user/.kube/config",
			want:       configNotKCS,
		},
		{
			name:       "session mode: unset KUBECONFIG",
			kcsSession: "1234",
			want:       configUnset,
		},
		{
			name:       "session mode: static path in KUBECONFIG",
			kcsSession: "1234",
			kubeconfig: staticPath,
			want:       configStaticInSession,
		},
		{
			name:       "session mode: static path among others",
			kcsSession: "1234",
			kubeconfig: staticPath + ":/home/user/.kube/config",
			want:       configStaticInSession,
		},
		{
			name:       "session mode: unrelated KUBECONFIG",
			kcsSession: "1234",
			kubeconfig: "/home/user/.kube/config",
			want:       configWrongSession,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("KUBECONFIG", tc.kubeconfig)
			os.Setenv("KCS_SESSION", tc.kcsSession)
			defer os.Unsetenv("KUBECONFIG")
			defer os.Unsetenv("KCS_SESSION")

			got := checkConfig(kubeDir)
			if got != tc.want {
				t.Errorf("checkConfig() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckConfigSessionOK(t *testing.T) {
	// Session path depends on SessionPath() which uses KCS_SESSION or ppid.
	// Set KCS_SESSION to a known value so we can construct the expected path.
	os.Setenv("KCS_SESSION", "testsession")
	defer os.Unsetenv("KCS_SESSION")

	sessionPath := "/run/user/1000/kcs/sessions/testsession"

	cases := []struct {
		name       string
		kubeconfig string
		want       configStatus
	}{
		{
			name:       "session path only",
			kubeconfig: sessionPath,
			want:       configOK,
		},
		{
			name:       "session path with extras",
			kubeconfig: sessionPath + ":/home/user/.kube/config",
			want:       configOK,
		},
	}

	// Override XDG_RUNTIME_DIR so SessionPath() returns a predictable value.
	os.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	defer os.Unsetenv("XDG_RUNTIME_DIR")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("KUBECONFIG", tc.kubeconfig)
			defer os.Unsetenv("KUBECONFIG")

			got := checkConfig("/home/user/.kube")
			if got != tc.want {
				t.Errorf("checkConfig() = %v, want %v", got, tc.want)
			}
		})
	}
}
