using System;
using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using System.Windows.Interop;
using System.Windows.Markup;
using System.Windows.Threading;
using Forms = System.Windows.Forms;

internal sealed class DesktopPanel : IDisposable
{
    private readonly Window panel = Load("Panel.xaml");
    private readonly Window alert = Load("PortAlert.xaml");
    private readonly ObservableCollection<ServiceRow> rows = new ObservableCollection<ServiceRow>();
    private readonly DispatcherTimer dismiss = new DispatcherTimer { Interval = TimeSpan.FromSeconds(5) };
    private readonly Action<bool> setLanguage;
    private readonly Func<string,string,string> translate;
    private Forms.Screen screen = Forms.Screen.PrimaryScreen;
    private Snapshot snapshot;
    private ServiceRow newest;
    private int newCount;

    private static Window Load(string name)
    {
        using (var data = Assembly.GetExecutingAssembly().GetManifestResourceStream(name))
            return (Window)XamlReader.Load(data);
    }

    public DesktopPanel(Func<string,string,string> translate, Action<bool> setLanguage, Action<string> open, Action cli, Action quit)
    {
        this.translate = translate;
        this.setLanguage = setLanguage;
        Part<ItemsControl>("Rows").ItemsSource = rows;
        Part<ItemsControl>("Rows").AddHandler(Button.ClickEvent, new RoutedEventHandler(delegate(object sender, RoutedEventArgs e) {
            var button = e.OriginalSource as Button;
            var row = button == null ? null : button.DataContext as ServiceRow;
            if (row != null && row.CanOpen) open(row.URL);
        }));
        Part<Button>("ClosePanel").Click += delegate { panel.Hide(); };
        ((Button)alert.FindName("CloseAlert")).Click += delegate { HideAlert(); };
        RoundCorners(panel);
        RoundCorners(alert);
        Part<Button>("CLI").Click += delegate { cli(); };
        Part<Button>("Quit").Click += delegate { quit(); };
        Part<Button>("Language").Click += delegate {
            var menu = new ContextMenu();
            foreach (var label in new [] {"English","繁體中文"}) {
                bool useChinese = label != "English";
                var item = new MenuItem { Header = label, IsChecked = label == translate("English","繁體中文") };
                item.Click += delegate { this.setLanguage(useChinese); };
                menu.Items.Add(item);
            }
            menu.PlacementTarget = Part<Button>("Language");
            menu.Placement = System.Windows.Controls.Primitives.PlacementMode.Bottom;
            menu.IsOpen = true;
        };
        panel.Deactivated += delegate { panel.Hide(); };
        panel.PreviewKeyDown += delegate(object sender, KeyEventArgs e) { if (e.Key == Key.Escape) { panel.Hide(); e.Handled = true; } };
        panel.Closing += delegate(object sender, CancelEventArgs e) { if (!disposing) { e.Cancel = true; panel.Hide(); } };
        panel.SizeChanged += delegate { if (panel.IsVisible) Position(panel); };
        alert.SourceInitialized += delegate {
            IntPtr h = new WindowInteropHelper(alert).Handle;
            SetWindowLong(h, -20, GetWindowLong(h, -20) | 0x08000000 | 0x80); // NOACTIVATE | TOOLWINDOW
        };
        alert.MouseLeftButtonUp += delegate { ShowPanel(screen); };
        dismiss.Tick += delegate { HideAlert(); };
        RefreshLanguage();
    }

    private T Part<T>(string name) where T : FrameworkElement { return (T)panel.FindName(name); }

    public void RefreshLanguage()
    {
        Part<TextBlock>("Heading").Text = translate("Local services","本機服務");
        Part<TextBlock>("AddressLabel").Text = translate("Address","位址");
        Part<TextBlock>("ServiceLabel").Text = translate("Service","服務");
        Part<TextBlock>("SourceLabel").Text = translate("Source","啟動來源");
        Part<TextBlock>("MemoryLabel").Text = translate("Memory","記憶體");
        var close = Part<Button>("ClosePanel");
        close.ToolTip = translate("Close panel","關閉面板");
        System.Windows.Automation.AutomationProperties.SetName(close, (string)close.ToolTip);
        var closeAlert = (Button)alert.FindName("CloseAlert");
        closeAlert.ToolTip = translate("Dismiss","關閉提示");
        System.Windows.Automation.AutomationProperties.SetName(closeAlert, (string)closeAlert.ToolTip);
        Part<Button>("Language").Content = translate("English","繁體中文") + "  ▾";
        Part<TextBlock>("CLIText").Text = translate("Open Ghosts CLI","開啟 Ghosts CLI");
        Part<Button>("Quit").Content = translate("Quit Ghosts","結束 Ghosts");
        if (panel.IsVisible) { Render(); Part<ItemsControl>("Rows").Items.Refresh(); }
        if (alert.IsVisible) AlertText();
    }

    public void Update(Snapshot next)
    {
        snapshot = next;
        if (panel.IsVisible) Render();
        if (next.Rows == null || !String.IsNullOrEmpty(next.Error)) return;
        ServiceRow[] fresh = Array.FindAll(next.Rows, r => r.NewPort);
        if (fresh.Length == 0) return;
        newest = fresh[0];
        newCount = fresh.Length;
        AlertText();
        Position(alert);
        alert.Show();
        Position(alert);
        dismiss.Stop();
        dismiss.Start();
    }

    private void AlertText()
    {
        ((TextBlock)alert.FindName("Greeting")).Text = translate("Ghost spotted!","撞著鬼！");
        ((TextBlock)alert.FindName("Detail")).Text = newest.DisplayName + "  ·  " + newest.Address + (newCount > 1 ? "  +" + (newCount - 1) : "");
        ((TextBlock)alert.FindName("Count")).Text = snapshot.Rows == null ? "–" : snapshot.Rows.Length.ToString();
    }

    private void HideAlert() { dismiss.Stop(); alert.Hide(); }

    public void ShowPanel(Forms.Screen target)
    {
        HideAlert();
        screen = target;
        Render();
        Position(panel);
        panel.Show();
        Position(panel);
        panel.Activate();
    }

    public void Error(string message)
    {
        ShowPanel(screen);
        Part<TextBlock>("Status").Text = message;
        Part<TextBlock>("Status").Visibility = Visibility.Visible;
    }

    private void Render()
    {
        ServiceRow[] next = snapshot == null || snapshot.Rows == null ? new ServiceRow[0] : snapshot.Rows;
        // Replace only changed rows so a background scan preserves scroll and keyboard navigation.
        for (int i = 0; i < next.Length; i++) {
            var row = next[i];
            row.SourceLabel = String.IsNullOrEmpty(row.Source) ? translate("Unknown","不明") : row.Source;
            if (i == rows.Count) rows.Add(row);
            else if (!rows[i].SameDisplay(row)) rows[i] = row;
        }
        while (rows.Count > next.Length) rows.RemoveAt(rows.Count - 1);
        Part<Grid>("Table").Visibility = rows.Count == 0 ? Visibility.Collapsed : Visibility.Visible;
        var status = Part<TextBlock>("Status");
        status.Text = snapshot == null ? translate("Scanning…","掃描中…") :
            !String.IsNullOrEmpty(snapshot.Error) ? snapshot.Error : translate("No local development services.","目前沒有開發服務在監聽連接埠。");
        status.Visibility = rows.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
        Part<Grid>("ColumnHeaders").Margin = new Thickness(18,9,18 + (rows.Count > 8 ? SystemParameters.VerticalScrollBarWidth : 0),4);
    }

    [DllImport("user32.dll")] private static extern bool SetWindowPos(IntPtr hwnd, IntPtr after, int x, int y, int cx, int cy, uint flags);
    [DllImport("user32.dll", EntryPoint="GetWindowLongW")] private static extern int GetWindowLong(IntPtr hwnd, int index);
    [DllImport("user32.dll", EntryPoint="SetWindowLongW")] private static extern int SetWindowLong(IntPtr hwnd, int index, int value);

    [DllImport("gdi32.dll")] private static extern IntPtr CreateRoundRectRgn(int x1,int y1,int x2,int y2,int width,int height);
    [DllImport("user32.dll")] private static extern int SetWindowRgn(IntPtr hwnd, IntPtr region, bool redraw);
    [DllImport("gdi32.dll")] private static extern bool DeleteObject(IntPtr region);

    private static void RoundCorners(Window window)
    {
        Action update = delegate {
            var source = PresentationSource.FromVisual(window) as HwndSource;
            if (source == null || window.ActualWidth < 1 || window.ActualHeight < 1) return;
            var scale = source.CompositionTarget.TransformToDevice;
            int w = (int)Math.Round(window.ActualWidth * scale.M11);
            int h = (int)Math.Round(window.ActualHeight * scale.M22);
            int diameter = (int)Math.Round(24 * scale.M11);
            IntPtr region = CreateRoundRectRgn(0,0,w+1,h+1,diameter,diameter);
            // Windows owns the region after a successful assignment.
            if (region != IntPtr.Zero && SetWindowRgn(source.Handle,region,true) == 0) DeleteObject(region);
        };
        window.SizeChanged += delegate { update(); };
        window.ContentRendered += delegate { update(); };
        window.DpiChanged += delegate { window.Dispatcher.BeginInvoke(update); };
    }

    private void Position(Window window)
    {
        var hwnd = new WindowInteropHelper(window).EnsureHandle();
        var transform = HwndSource.FromHwnd(hwnd).CompositionTarget.TransformToDevice;
        var bounds = screen.WorkingArea;
        window.MaxWidth = Math.Max(1, bounds.Width / transform.M11 - 16);
        window.UpdateLayout();
        int width = (int)Math.Ceiling(window.ActualWidth * transform.M11);
        int height = (int)Math.Ceiling(window.ActualHeight * transform.M22);
        int margin = (int)Math.Ceiling(8 * transform.M11);
        bool left = bounds.Left > screen.Bounds.Left, top = bounds.Top > screen.Bounds.Top;
        int x = left ? bounds.Left + margin : Math.Max(bounds.Left, bounds.Right - width - margin);
        int y = top ? bounds.Top + margin : Math.Max(bounds.Top, bounds.Bottom - height - margin);
        if (window == alert && panel.IsVisible) {
            int panelHeight = (int)Math.Ceiling(panel.ActualHeight * transform.M22);
            y = top ? y + panelHeight + margin : Math.Max(bounds.Top, y - panelHeight - margin);
        }
        SetWindowPos(hwnd, IntPtr.Zero, x,y,0,0,0x15); // NOSIZE | NOZORDER | NOACTIVATE
    }

    private bool disposing;
    public void Dispose()
    {
        disposing = true;
        dismiss.Stop();
        alert.Close();
        panel.Close();
    }
}
