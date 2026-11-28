using System;
using System.Diagnostics;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Globalization;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using System.Web.Script.Serialization;
using System.Windows.Forms;

internal sealed class ServiceRow
{
    public string Name { get; set; }
    public string Address { get; set; }
    public string Title { get; set; }
    public string Source { get; set; }
    public string Service { get; set; }
    public string URL { get; set; }
    public long Memory { get; set; }
    public bool NewPort { get; set; }
    public string SourceLabel { get; set; }
    public string DisplayName { get { return !String.IsNullOrEmpty(Title) ? Title : !String.IsNullOrEmpty(Service) ? Service : Name; } }
    public string Hint { get { return Address + " · " + DisplayName + " · " + SourceLabel; } }
    public string MemoryLabel { get { return Memory >= 1073741824 ? (Memory / 1073741824.0).ToString("0.#") + " GB" : (Memory / 1048576.0).ToString("0.#") + " MB"; } }
    public bool CanOpen { get {
        Uri uri;
        return Uri.TryCreate(URL, UriKind.Absolute, out uri) && (uri.Scheme == "http" || uri.Scheme == "https");
    } }
    public bool SameDisplay(ServiceRow r) {
        return Address == r.Address && DisplayName == r.DisplayName && SourceLabel == r.SourceLabel && MemoryLabel == r.MemoryLabel && URL == r.URL;
    }
}

internal sealed class Snapshot
{
    public ServiceRow[] Rows { get; set; }
    public string Error { get; set; }
}

internal sealed class Tray : ApplicationContext
{
    private readonly string cli = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "ghosts.exe");
    private readonly string preference = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "Ghosts", "language.txt");
    private readonly DesktopPanel desktop;
    private readonly Control dispatch = new Control();
    private readonly NotifyIcon notification = new NotifyIcon();
    private readonly Icon icon = MakeIcon();
    private readonly System.Windows.Forms.Timer reconnect = new System.Windows.Forms.Timer { Interval = 5000 };
    private readonly JavaScriptSerializer json = new JavaScriptSerializer { MaxJsonLength = 8 * 1024 * 1024 };
    private Process worker;
    private Snapshot snapshot;
    private bool chinese;
    private bool closed;

    [DllImport("user32.dll")]
    private static extern bool DestroyIcon(IntPtr handle);

    private static Icon MakeIcon()
    {
        using (Bitmap bmp = new Bitmap(32, 32))
        using (Graphics g = Graphics.FromImage(bmp))
        using (GraphicsPath ghost = new GraphicsPath())
        {
            g.SmoothingMode = SmoothingMode.AntiAlias;
            ghost.AddArc(4, 2, 24, 24, 180, 180);
            ghost.AddLines(new Point[] { new Point(28, 14), new Point(28, 29), new Point(22, 26), new Point(16, 30), new Point(10, 26), new Point(4, 29) });
            ghost.CloseFigure();
            g.FillPath(Brushes.White, ghost);
            g.DrawPath(Pens.DimGray, ghost);
            g.FillEllipse(Brushes.Black, 10, 12, 4, 6);
            g.FillEllipse(Brushes.Black, 19, 12, 4, 6);
            IntPtr handle = bmp.GetHicon();
            try { using (Icon temp = Icon.FromHandle(handle)) return (Icon)temp.Clone(); }
            finally { DestroyIcon(handle); }
        }
    }

    private string T(string en, string zh) { return chinese ? zh : en; }

    internal Tray()
    {
        chinese = CultureInfo.CurrentUICulture.Name.StartsWith("zh", StringComparison.OrdinalIgnoreCase);
        try { if (File.Exists(preference)) chinese = File.ReadAllText(preference).Trim() == "zh-TW"; }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }
        dispatch.CreateControl();
        notification.Icon = icon;
        notification.Text = "Ghosts";
        desktop = new DesktopPanel(T, SetLanguage, Open, OpenTerminal, ExitThread);
        notification.MouseClick += delegate { desktop.ShowPanel(Screen.FromPoint(Cursor.Position)); };
        reconnect.Tick += delegate { reconnect.Stop(); StartWorker(); };
        notification.Visible = true;
        StartWorker();
        ShowPanel();
    }

    public void ShowPanel() { desktop.ShowPanel(Screen.FromPoint(Cursor.Position)); }

    private void StartWorker()
    {
        Process p = new Process();
        p.StartInfo = new ProcessStartInfo(cli, "-tray-feed") {
            UseShellExecute = false, CreateNoWindow = true,
            RedirectStandardInput = true, RedirectStandardOutput = true, StandardOutputEncoding = Encoding.UTF8
        };
        worker = p;
        p.OutputDataReceived += delegate(object sender, DataReceivedEventArgs e) {
            if (closed) return;
            try { dispatch.BeginInvoke((Action)delegate {
                if (closed || p != worker) return;
                if (e.Data == null) {
                    WorkerEnded();
                    return;
                }
                try { Apply(json.Deserialize<Snapshot>(e.Data)); }
                catch (ArgumentException err) { Apply(new Snapshot { Error = err.Message }); }
            }); } catch (InvalidOperationException) { }
        };
        try { p.Start(); p.BeginOutputReadLine(); }
        catch (System.ComponentModel.Win32Exception err) {
            WorkerEnded();
            Apply(new Snapshot { Error = err.Message });
        }
    }

    private void WorkerEnded()
    {
        worker.Dispose();
        worker = null;
        Apply(new Snapshot { Error = T("Scanner disconnected", "掃描器已中斷連線") });
        reconnect.Start();
    }

    private void Apply(Snapshot next)
    {
        snapshot = next;
        desktop.Update(next);
        if (next.Rows == null || !String.IsNullOrEmpty(next.Error)) {
            notification.Text = T("Ghosts · Scanner unavailable", "Ghosts · 掃描器無法使用");
            return;
        }
        notification.Text = "Ghosts · " + next.Rows.Length + T(" development ports", " 個開發連接埠");
    }

    private void SetLanguage(bool useChinese)
    {
        chinese = useChinese;
        desktop.RefreshLanguage();
        try {
            Directory.CreateDirectory(Path.GetDirectoryName(preference));
            File.WriteAllText(preference, chinese ? "zh-TW" : "en");
        } catch (Exception err) { ShowError(err); }
        if (snapshot != null && snapshot.Rows != null)
            notification.Text = "Ghosts · " + snapshot.Rows.Length + T(" development ports", " 個開發連接埠");
    }

    private void ShowError(Exception err)
    {
        desktop.Error(err.Message);
    }

    private void Open(string target)
    {
        try { Process.Start(new ProcessStartInfo(target) { UseShellExecute = true }); }
        catch (System.ComponentModel.Win32Exception err) { ShowError(err); }
    }

    private void OpenTerminal()
    {
        string terminal = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), @"Microsoft\WindowsApps\wt.exe");
        bool hasTerminal = File.Exists(terminal);
        var info = new ProcessStartInfo {
            FileName = hasTerminal ? terminal : cli,
            Arguments = hasTerminal ? "-w new --size 148,34 new-tab --title Ghosts \"" + cli + "\"" : "",
            WorkingDirectory = Path.GetDirectoryName(cli), UseShellExecute = false
        };
        // The tray may be launched by a headless tool. Start its interactive console with
        // desktop user preferences, independent of the launcher's output-capture settings.
        foreach (string name in new [] { "NO_COLOR", "TERM", "COLORTERM", "CLICOLOR" }) {
            string value = Environment.GetEnvironmentVariable(name, EnvironmentVariableTarget.User)
                ?? Environment.GetEnvironmentVariable(name, EnvironmentVariableTarget.Machine);
            if (String.IsNullOrEmpty(value)) info.EnvironmentVariables.Remove(name);
            else info.EnvironmentVariables[name] = value;
        }
        try { Process.Start(info); }
        catch (System.ComponentModel.Win32Exception err) { ShowError(err); }
    }

    protected override void ExitThreadCore()
    {
        closed = true;
        reconnect.Dispose();
        if (worker != null) { worker.StandardInput.Close(); worker.Dispose(); }
        notification.Visible = false;
        notification.Dispose();
        desktop.Dispose();
        icon.Dispose();
        dispatch.Dispose();
        base.ExitThreadCore();
        System.Windows.Application.Current.Shutdown();
    }

    [STAThread]
    private static void Main()
    {
        bool first;
        using (var mutex = new Mutex(true, @"Local\Ghosts.Tray", out first)) {
            using (var show = new EventWaitHandle(false, EventResetMode.AutoReset, @"Local\Ghosts.ShowPanel")) {
                if (!first) { show.Set(); return; }
                Application.EnableVisualStyles();
                var app = new System.Windows.Application { ShutdownMode = System.Windows.ShutdownMode.OnExplicitShutdown };
                var tray = new Tray();
                var registration = ThreadPool.RegisterWaitForSingleObject(show, delegate {
                    app.Dispatcher.BeginInvoke((Action)tray.ShowPanel);
                }, null, Timeout.Infinite, false);
                try { app.Run(); } finally { registration.Unregister(null); }
            }
        }
    }
}
