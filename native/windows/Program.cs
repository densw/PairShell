using System.Diagnostics;
using System.Text;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;

internal static class Program {
    [STAThread] static void Main() {
        var runtime = Path.Combine(AppContext.BaseDirectory, "runtime");
        if (!File.Exists(Path.Combine(runtime, "WebView2Loader.dll"))) runtime = AppContext.BaseDirectory;
        if (File.Exists(Path.Combine(runtime, "WebView2Loader.dll"))) CoreWebView2Environment.SetLoaderDllFolderPath(runtime);
        ApplicationConfiguration.Initialize();
        Application.Run(new CustomerWindow());
    }
}
internal sealed class CustomerWindow : Form {
    readonly WebView2 web = new() { Dock = DockStyle.Fill };
    readonly HashSet<string> origins = new(StringComparer.Ordinal);
    readonly string profile = Path.Combine(Path.GetTempPath(), "PairShell-Window-" + Guid.NewGuid());
    Process? backend;
    bool loaded, closing, canClose;
    void Trace(string message) {
        var result=Environment.GetEnvironmentVariable("PAIRSHELL_SMOKE_RESULT");
        if(!string.IsNullOrEmpty(result)) File.AppendAllText(result+".diagnostics",message+Environment.NewLine);
    }
    public CustomerWindow() {
        if (Environment.ProcessPath is { } executable) Icon = Icon.ExtractAssociatedIcon(executable);
        Text = "PairShell"; var area=Screen.PrimaryScreen!.WorkingArea; Width = Math.Min(1320,area.Width-60); Height = Math.Min(1050,area.Height-60);
        MinimumSize = new Size(700, 550); StartPosition = FormStartPosition.CenterScreen;
        Controls.Add(web); Shown += async (_, _) => await Start(); FormClosing += Stop;
    }
    async Task Start() {
        try {
            Trace("initializing");
            var env = await CoreWebView2Environment.CreateAsync(null, profile);
            await web.EnsureCoreWebView2Async(env);
            Trace("webview ready");
            web.CoreWebView2.Settings.AreDevToolsEnabled = false;
            web.CoreWebView2.Settings.AreDefaultContextMenusEnabled = false;
            web.CoreWebView2.Settings.AreBrowserAcceleratorKeysEnabled = false;
            web.CoreWebView2.NavigationStarting += (_, e) => {
                if (!Uri.TryCreate(e.Uri, UriKind.Absolute, out var u) || !origins.Contains(u.GetLeftPart(UriPartial.Authority))) e.Cancel = true;
            };
            web.CoreWebView2.NewWindowRequested += (_, e) => e.Handled = true;
            web.CoreWebView2.DownloadStarting += (_, e) => e.Cancel = true;
            web.CoreWebView2.PermissionRequested += (_, e) => {
                // Clipboard reads only follow an explicit local-window paste action.
                bool local = Uri.TryCreate(e.Uri, UriKind.Absolute, out var u) && origins.Contains(u.GetLeftPart(UriPartial.Authority));
                e.State = local && e.IsUserInitiated && e.PermissionKind == CoreWebView2PermissionKind.ClipboardRead
                    ? CoreWebView2PermissionState.Allow : CoreWebView2PermissionState.Deny;
            };
            web.CoreWebView2.NavigationCompleted += async (_, e) => {
                Trace("navigation completed: "+e.IsSuccess+" "+e.WebErrorStatus);
 Text=web.Source?.AbsolutePath=="/merchant" ? "PairShell · 发起协助" : "PairShell · 接受协助";
                if (Environment.GetCommandLineArgs().Contains("--layout-test") && e.IsSuccess) {
                    await Task.Delay(250); // Read settled geometry; never alter page content or authorization.
                    string geometry = await web.CoreWebView2.ExecuteScriptAsync("(() => {const r=document.documentElement;return {width:innerWidth,height:innerHeight,scrollWidth:r.scrollWidth,scrollHeight:r.scrollHeight,passed:r.scrollWidth<=innerWidth+1&&r.scrollHeight<=innerHeight+1};})()");
                    bool passed = System.Text.Json.JsonDocument.Parse(geometry).RootElement.GetProperty("passed").GetBoolean();
                    var result = Environment.GetEnvironmentVariable("PAIRSHELL_SMOKE_RESULT");
                    if (!string.IsNullOrEmpty(result)) {
                        File.WriteAllText(result+".json",geometry);
                        using (var file = File.Create(result+".png")) await web.CoreWebView2.CapturePreviewAsync(CoreWebView2CapturePreviewImageFormat.Png,file);
                        File.WriteAllText(result,passed ? "WINDOW_LAYOUT_READY" : "WINDOW_LAYOUT_FAILED");
                    }
                    Close(); return;
                }
                if (Environment.GetCommandLineArgs().Contains("--smoke-test") && e.IsSuccess) {
                    string title = await web.CoreWebView2.ExecuteScriptAsync("document.querySelector('.brand')?.textContent || document.querySelector('h1')?.textContent");
                    if (title.Contains("PairShell")) {
                        var result = Environment.GetEnvironmentVariable("PAIRSHELL_SMOKE_RESULT");
                        if (!string.IsNullOrEmpty(result)) {
                            using (var file = File.Create(result+".png")) await web.CoreWebView2.CapturePreviewAsync(CoreWebView2CapturePreviewImageFormat.Png,file);
                            File.WriteAllText(result, "WINDOW_SMOKE_READY");
                        }
                    }
                    if(title.Contains("PairShell")) Close();
                }
            };
            var backendPath = Path.Combine(AppContext.BaseDirectory, "runtime", "PairShellBackend.exe");
            if (!File.Exists(backendPath)) backendPath = Path.Combine(AppContext.BaseDirectory, "PairShellBackend.exe");
            var info = new ProcessStartInfo(backendPath) {
                UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true,
                RedirectStandardError = true, RedirectStandardInput = true, StandardOutputEncoding = Encoding.UTF8
            };
            info.Environment["PAIRSHELL_DESKTOP_EMBEDDED"] = "1";
            info.Environment["PAIRSHELL_PARENT_STDIN"] = "1";
            backend = new Process { StartInfo = info, EnableRaisingEvents = true };
            backend.OutputDataReceived += (_, e) => {
                const string prefix = "PAIRSHELL_WINDOW_URL=";
                if (e.Data == null || e.Data.Length > 8192 || !e.Data.StartsWith(prefix)) return;
                if (!Uri.TryCreate(e.Data[prefix.Length..], UriKind.Absolute, out var u) || u.Scheme != "http" || u.Host != "127.0.0.1" || u.Port < 1 || u.UserInfo != "") return;
                Trace("backend origin accepted");
                if (!IsDisposed) BeginInvoke(() => {
                    origins.Add(u.GetLeftPart(UriPartial.Authority));
                    if (!loaded) { loaded = true; var target=new UriBuilder(u);if(Environment.GetCommandLineArgs().Contains("--merchant") || string.Equals(Path.GetFileNameWithoutExtension(Environment.ProcessPath),"PairShell.Merchant",StringComparison.OrdinalIgnoreCase))target.Path="/merchant";web.CoreWebView2.Navigate(target.Uri.AbsoluteUri); }
                });
            };
            backend.ErrorDataReceived += (_, _) => { }; // Never persist invites/control tokens.
            backend.Exited += (_, _) => { if (!IsDisposed && IsHandleCreated) BeginInvoke(() => Text = "PairShell · 已结束"); };
            backend.Start(); backend.BeginOutputReadLine(); backend.BeginErrorReadLine();
        } catch (Exception ex) {
            if(Environment.GetCommandLineArgs().Contains("--smoke-test")) {
                var result=Environment.GetEnvironmentVariable("PAIRSHELL_SMOKE_RESULT");
                if(!string.IsNullOrEmpty(result)) File.WriteAllText(result,"WINDOW_SMOKE_FAILED: "+ex.ToString());
                Close(); return;
            }
            MessageBox.Show(ex is WebView2RuntimeNotFoundException ? "需要安装 Microsoft Edge WebView2 Runtime 后才能打开客户窗口。" : "客户窗口启动失败，请确认程序文件完整。", "PairShell");
            Close();
        }
    }
    async void Stop(object? sender, FormClosingEventArgs e) {
        if (canClose) return;
        e.Cancel = true; if (closing) return; closing = true; Enabled = false;
        try {
            if (backend is { HasExited: false }) {
                backend.StandardInput.Close(); // EOF asks Go to revoke/clean up before exiting.
                try { await backend.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(12)); }
                catch (TimeoutException) { backend.Kill(true); await backend.WaitForExitAsync(); }
            }
        } finally {
            web.Dispose(); backend?.Dispose();
            try { Directory.Delete(profile, true); } catch (IOException) { } catch (UnauthorizedAccessException) { }
            canClose = true; Close();
        }
    }
}
