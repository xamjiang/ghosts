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
    private readonly ContextMenuStrip menu = new ContextMenuStrip();
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

    private Tray()
    {
        chinese = CultureInfo.CurrentUICulture.Name.StartsWith("zh", StringComparison.OrdinalIgnoreCase);
        try { if (File.Exists(preference)) chinese = File.ReadAllText(preference).Trim() == "zh-TW"; }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }
        dispatch.CreateControl();
        notification.Icon = icon;
        notification.Text = "Ghosts";
        notification.ContextMenuStrip = menu;
        menu.Items.Add("Ghosts");
        menu.Opening += delegate { BuildMenu(); };
        notification.MouseClick += delegate(object sender, MouseEventArgs e) {
            if (e.Button == MouseButtons.Left) menu.Show(Cursor.Position);
        };
        reconnect.Tick += delegate { reconnect.Stop(); StartWorker(); };
        notification.Visible = true;
        StartWorker();
    }

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
        if (next.Rows == null || !String.IsNullOrEmpty(next.Error)) {
            notification.Text = T("Ghosts · Scanner unavailable", "Ghosts · 掃描器無法使用");
            return;
        }
        notification.Text = "Ghosts · " + next.Rows.Length + T(" development ports", " 個開發連接埠");
        var alerts = Array.FindAll(next.Rows, r => r.NewPort);
        if (alerts.Length > 0) {
            ServiceRow first = alerts[0];
            string more = alerts.Length > 1 ? " (+" + (alerts.Length - 1) + ")" : "";
            notification.ShowBalloonTip(5000, T("Ghost spotted!", "撞著鬼！"), first.Name + " · " + first.Address + more, ToolTipIcon.Info);
        }
    }

    private void SetLanguage(bool useChinese)
    {
        chinese = useChinese;
        try {
            Directory.CreateDirectory(Path.GetDirectoryName(preference));
            File.WriteAllText(preference, chinese ? "zh-TW" : "en");
        } catch (Exception err) { ShowError(err); }
        if (snapshot != null && snapshot.Rows != null)
            notification.Text = "Ghosts · " + snapshot.Rows.Length + T(" development ports", " 個開發連接埠");
    }

    private void ShowError(Exception err)
    {
        notification.ShowBalloonTip(5000, "Ghosts", err.Message, ToolTipIcon.Warning);
    }

    private void Open(string target)
    {
        try { Process.Start(new ProcessStartInfo(target) { UseShellExecute = true }); }
        catch (System.ComponentModel.Win32Exception err) { ShowError(err); }
    }

    private void BuildMenu()
    {
        while (menu.Items.Count > 0) menu.Items[0].Dispose();
        if (snapshot == null || !String.IsNullOrEmpty(snapshot.Error) || snapshot.Rows.Length == 0) {
            string status = snapshot == null ? T("Scanning…", "掃描中…") :
                !String.IsNullOrEmpty(snapshot.Error) ? snapshot.Error : T("No development ports", "目前沒有開發連接埠");
            menu.Items.Add(status).Enabled = false;
        } else {
            foreach (ServiceRow row in snapshot.Rows) {
                string label = String.IsNullOrEmpty(row.Title) ? row.Name : row.Title;
                string details = row.Address + " · " + (row.Memory / 1048576.0).ToString("0.#") + " MB";
                if (!String.IsNullOrEmpty(row.Source)) details += " · " + row.Source;
                if (!String.IsNullOrEmpty(row.Service)) details += " · " + row.Service;
                var item = new ToolStripMenuItem((label + Environment.NewLine + details).Replace("&", "&&"));
                Uri url;
                if (Uri.TryCreate(row.URL, UriKind.Absolute, out url) && (url.Scheme == "https" || url.Scheme == "http")) {
                    string target = url.AbsoluteUri;
                    item.Click += delegate { Open(target); };
                } else item.Enabled = false;
                menu.Items.Add(item);
            }
        }
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add(T("Open Ghosts CLI", "開啟 Ghosts CLI"), null, delegate { Open(cli); });
        var language = new ToolStripMenuItem(T("Language", "語言"));
        ((ToolStripMenuItem)language.DropDownItems.Add("English", null, delegate { SetLanguage(false); })).Checked = !chinese;
        ((ToolStripMenuItem)language.DropDownItems.Add("繁體中文", null, delegate { SetLanguage(true); })).Checked = chinese;
        menu.Items.Add(language);
        menu.Items.Add(T("Quit", "結束"), null, delegate { ExitThread(); });
    }

    protected override void ExitThreadCore()
    {
        closed = true;
        reconnect.Dispose();
        if (worker != null) { worker.StandardInput.Close(); worker.Dispose(); }
        notification.Visible = false;
        notification.Dispose();
        menu.Dispose();
        icon.Dispose();
        dispatch.Dispose();
        base.ExitThreadCore();
    }

    [STAThread]
    private static void Main()
    {
        bool first;
        using (var mutex = new Mutex(true, @"Local\Ghosts.Tray", out first)) {
            if (!first) return;
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            Application.Run(new Tray());
        }
    }
}
