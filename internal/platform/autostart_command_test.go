package platform

import "testing"

func TestQuoteWindowsArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "plain",
			arg:  `C:\Tools\tg-fyne-proxy.exe`,
			want: `C:\Tools\tg-fyne-proxy.exe`,
		},
		{
			name: "spaces",
			arg:  `C:\Program Files\TG Fyne Proxy\tg-fyne-proxy.exe`,
			want: `"C:\Program Files\TG Fyne Proxy\tg-fyne-proxy.exe"`,
		},
		{
			name: "cyrillic path",
			arg:  `C:\Пользователи\falearn\TG Fyne Proxy\tg-fyne-proxy.exe`,
			want: `"C:\Пользователи\falearn\TG Fyne Proxy\tg-fyne-proxy.exe"`,
		},
		{
			name: "trailing slash",
			arg:  `C:\Program Files\TG Fyne Proxy\`,
			want: `"C:\Program Files\TG Fyne Proxy\\"`,
		},
		{
			name: "quote",
			arg:  `C:\Apps\TG "Proxy"\tg-fyne-proxy.exe`,
			want: `"C:\Apps\TG \"Proxy\"\tg-fyne-proxy.exe"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteWindowsArg(tt.arg); got != tt.want {
				t.Fatalf("quoteWindowsArg(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}

func TestAutostartCommand(t *testing.T) {
	exe := `C:\Путь With Space\tg-fyne-proxy.exe`
	want := `"C:\Путь With Space\tg-fyne-proxy.exe" --autostart`
	if got := autostartCommand(exe); got != want {
		t.Fatalf("autostartCommand() = %q, want %q", got, want)
	}
}

func TestLegacyAutostartCommand(t *testing.T) {
	exe := `C:\Tools\tg-fyne-proxy.exe`
	want := `"C:\\Tools\\tg-fyne-proxy.exe"`
	if got := legacyAutostartCommand(exe); got != want {
		t.Fatalf("legacyAutostartCommand() = %q, want %q", got, want)
	}
}

func TestIsAutostartLaunch(t *testing.T) {
	if !IsAutostartLaunch([]string{"--other", AutostartFlag}) {
		t.Fatal("expected autostart launch")
	}
	if IsAutostartLaunch([]string{"--other"}) {
		t.Fatal("unexpected autostart launch")
	}
}
