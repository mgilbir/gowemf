// Windows oracle (tools/windows/oracle.ps1): extracts GDI+ hatch patterns,
// Windows double-byte decoding, GDI+-recorded metafiles, and GDI/GDI+
// renderings of generated metafiles. Compiled by Windows PowerShell 5.1's
// Add-Type (C# 5).
using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;

public static class WinOracle
{
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }

    [DllImport("gdi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    static extern IntPtr GetEnhMetaFileW(string name);
    [DllImport("gdi32.dll", SetLastError = true)]
    static extern bool PlayEnhMetaFile(IntPtr hdc, IntPtr hemf, ref RECT rect);
    [DllImport("gdi32.dll")]
    static extern bool DeleteEnhMetaFile(IntPtr hemf);
    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    static extern int MultiByteToWideChar(uint codePage, uint flags, byte[] mb, int cb, [Out] char[] wide, int cch);

    static string Row(Bitmap bmp, int ox, int oy, int y)
    {
        var sb = new StringBuilder();
        for (int x = 0; x < 8; x++)
        {
            sb.Append(bmp.GetPixel(ox + x, oy + y).R < 128 ? '1' : '0');
        }
        return sb.ToString();
    }

    // Hatches fills 32x32 pixels with each HatchStyle (black on white,
    // rendering origin at 0,0) and reports the 8x8 cell at the origin, whether
    // the fill repeats with period 8, and whether every pixel is pure black or
    // white. Styles 9 and 25 are repeated with the rendering origin at (3,5).
    public static string Hatches()
    {
        var sb = new StringBuilder();
        for (int s = 0; s <= 52; s++)
        {
            sb.Append(Hatch(s, 0, 0));
        }
        sb.Append(ColorHatch(2));
        sb.Append(ColorHatch(5));
        sb.Append(Hatch(9, 3, 5));
        sb.Append(Hatch(25, 3, 5));
        return sb.ToString();
    }

    // ColorHatch reports the first row of a red on blue hatch, to show how
    // anti-aliased pattern pixels blend the two colors.
    static string ColorHatch(int style)
    {
        using (var bmp = new Bitmap(16, 16, PixelFormat.Format24bppRgb))
        using (var g = Graphics.FromImage(bmp))
        using (var brush = new HatchBrush((HatchStyle)style, Color.Red, Color.Blue))
        {
            g.FillRectangle(brush, 0, 0, 16, 16);
            var sb = new StringBuilder("color style=" + style + " row0=");
            for (int x = 0; x < 8; x++)
            {
                Color c = bmp.GetPixel(x, 0);
                sb.Append(string.Format("{0:x2}{1:x2}{2:x2} ", c.R, c.G, c.B));
            }
            return sb.ToString().TrimEnd() + "\n";
        }
    }

    static string Hatch(int style, int ox, int oy)
    {
        using (var bmp = new Bitmap(32, 32, PixelFormat.Format24bppRgb))
        using (var g = Graphics.FromImage(bmp))
        using (var brush = new HatchBrush((HatchStyle)style, Color.Black, Color.White))
        {
            g.SmoothingMode = SmoothingMode.None;
            g.RenderingOrigin = new Point(ox, oy);
            g.FillRectangle(brush, 0, 0, 32, 32);
            bool periodic = true, binary = true;
            for (int y = 0; y < 24; y++)
            {
                for (int x = 0; x < 24; x++)
                {
                    Color c = bmp.GetPixel(x, y);
                    if (c.ToArgb() != Color.Black.ToArgb() && c.ToArgb() != Color.White.ToArgb())
                    {
                        binary = false;
                    }
                    if (c.ToArgb() != bmp.GetPixel(x + 8, y).ToArgb() || c.ToArgb() != bmp.GetPixel(x, y + 8).ToArgb())
                    {
                        periodic = false;
                    }
                }
            }
            var rows = new string[8];
            var gray = new StringBuilder();
            for (int y = 0; y < 8; y++)
            {
                rows[y] = Row(bmp, 0, 0, y);
                for (int x = 0; x < 8; x++)
                {
                    Color c = bmp.GetPixel(x, y);
                    gray.Append(string.Format("{0:x2}{1:x2}{2:x2}", c.R, c.G, c.B));
                    gray.Append(x == 7 ? (y == 7 ? "" : "/") : " ");
                }
            }
            return string.Format("style={0} origin={1},{2} periodic={3} binary={4} rows={5} rgb={6}\n",
                style, ox, oy, periodic, binary, string.Join(",", rows), gray);
        }
    }

    static string Decode(uint cp, byte[] b)
    {
        var w = new char[16];
        int n = MultiByteToWideChar(cp, 0, b, b.Length, w, w.Length);
        var sb = new StringBuilder();
        for (int i = 0; i < n; i++)
        {
            if (i > 0) sb.Append(' ');
            sb.Append(((int)w[i]).ToString("x4"));
        }
        return sb.ToString();
    }

    // DumpCodePage decodes every byte alone, every byte followed by 'A', and
    // every lead/trail pair followed by 'A', so the consumption of invalid
    // sequences shows.
    public static void DumpCodePage(uint cp, string path)
    {
        using (var w = new StreamWriter(path, false, Encoding.ASCII))
        {
            for (int b = 0; b < 256; b++)
            {
                w.WriteLine(string.Format("{0:x2}: {1} | {2}", b, Decode(cp, new byte[] { (byte)b }), Decode(cp, new byte[] { (byte)b, 0x41 })));
            }
            for (int lead = 0x80; lead < 256; lead++)
            {
                for (int trail = 0; trail < 256; trail++)
                {
                    w.WriteLine(string.Format("{0:x2}{1:x2}: {2}", lead, trail, Decode(cp, new byte[] { (byte)lead, (byte)trail, 0x41 })));
                }
            }
        }
    }

    delegate void Draw(Graphics g);

    // Record writes an EMF+ Only file of w x h pixels drawn by GDI+ itself.
    static StringBuilder recordErrors = new StringBuilder();

    static void Record(string path, int w, int h, Draw draw)
    {
        try
        {
            RecordOne(path, w, h, draw);
        }
        catch (Exception e)
        {
            recordErrors.AppendLine(Path.GetFileName(path) + ": " + e.Message);
        }
    }

    static void RecordOne(string path, int w, int h, Draw draw)
    {
        using (var refBmp = new Bitmap(1, 1))
        using (var rg = Graphics.FromImage(refBmp))
        {
            IntPtr hdc = rg.GetHdc();
            try
            {
                using (var mf = new Metafile(path, hdc, new RectangleF(0, 0, w, h), MetafileFrameUnit.Pixel, EmfType.EmfPlusOnly))
                using (var g = Graphics.FromImage(mf))
                {
                    g.PageUnit = GraphicsUnit.Pixel;
                    draw(g);
                }
            }
            finally
            {
                rg.ReleaseHdc(hdc);
            }
        }
    }

    static Bitmap Quadrants(int n)
    {
        var bmp = new Bitmap(n, n, PixelFormat.Format32bppArgb);
        for (int y = 0; y < n; y++)
        {
            for (int x = 0; x < n; x++)
            {
                Color c = y < n / 2 ? (x < n / 2 ? Color.Red : Color.Lime) : (x < n / 2 ? Color.Blue : Color.Yellow);
                bmp.SetPixel(x, y, c);
            }
        }
        return bmp;
    }

    // RecordScenes writes metafiles recorded by GDI+ to dir, each settling
    // how the real writer encodes a feature and how GDI+ draws it.
    public static string RecordScenes(string dir, string inputs)
    {
        recordErrors.Clear();
        foreach (string name in new string[] { "win-inner.emf", "win-inner.wmf", "win-inner-1440.wmf" })
        {
            string src = Path.Combine(inputs, name);
            Record(Path.Combine(dir, "rec-embed-" + name.Replace('.', '-') + ".emf"), 96, 64, delegate(Graphics g)
            {
                using (var mf = new Metafile(src))
                {
                    g.DrawImage(mf, new Rectangle(4, 4, 80, 40));
                }
            });
        }
        Record(Path.Combine(dir, "rec-hatch-color.emf"), 96, 64, delegate(Graphics g)
        {
            using (var b = new HatchBrush(HatchStyle.ForwardDiagonal, Color.Red, Color.Blue))
            {
                g.FillRectangle(b, 0, 0, 32, 32);
            }
            using (var b = new HatchBrush(HatchStyle.Percent50, Color.FromArgb(128, 255, 0, 0), Color.FromArgb(255, 0, 0, 255)))
            {
                g.FillRectangle(b, 40, 0, 32, 32);
            }
        });
        string inner = Path.Combine(dir, "rec-inner.emf");
        Record(inner, 40, 20, delegate(Graphics g)
        {
            g.FillRectangle(Brushes.Red, 2, 2, 16, 16);
            g.FillRectangle(Brushes.Blue, 21, 2, 17, 16);
        });
        Record(Path.Combine(dir, "rec-image-bitmap.emf"), 96, 64, delegate(Graphics g)
        {
            using (var q = Quadrants(16))
            {
                g.InterpolationMode = InterpolationMode.NearestNeighbor;
                g.DrawImage(q, new Rectangle(8, 8, 32, 32));
                g.DrawImage(q, new Point[] { new Point(56, 8), new Point(88, 8), new Point(48, 40) });
                g.DrawImage(q, new RectangleF(8, 44, 24, 16), new RectangleF(3.5f, 3.5f, 6, 4), GraphicsUnit.Pixel);
            }
        });
        Record(Path.Combine(dir, "rec-image-png.emf"), 96, 64, delegate(Graphics g)
        {
            using (var q = Quadrants(16))
            using (var ms = new MemoryStream())
            {
                q.Save(ms, ImageFormat.Png);
                ms.Position = 0;
                using (var png = Image.FromStream(ms))
                {
                    g.DrawImage(png, new Rectangle(8, 8, 32, 32));
                }
            }
        });
        Record(Path.Combine(dir, "rec-image-metafile.emf"), 96, 64, delegate(Graphics g)
        {
            using (var mf = new Metafile(inner))
            {
                g.DrawImage(mf, new Rectangle(4, 4, 80, 40));
                g.DrawImage(mf, new RectangleF(50, 44, 40, 16), new RectangleF(10, 0, 30, 20), GraphicsUnit.Pixel);
            }
        });
        Record(Path.Combine(dir, "rec-image-outside.emf"), 96, 64, delegate(Graphics g)
        {
            using (var mf = new Metafile(inner))
            using (var q = Quadrants(16))
            {
                g.DrawImage(mf, new RectangleF(4, 4, 56, 20), new RectangleF(-10, 0, 60, 20), GraphicsUnit.Pixel);
                g.DrawImage(q, new RectangleF(64, 4, 24, 24), new RectangleF(-4, -4, 24, 24), GraphicsUnit.Pixel);
                using (var attrs = new ImageAttributes())
                {
                    attrs.SetWrapMode(WrapMode.Clamp, Color.Transparent);
                    g.DrawImage(mf, new Rectangle(4, 36, 56, 20), -10, 0, 60, 20, GraphicsUnit.Pixel, attrs);
                    g.DrawImage(q, new Rectangle(64, 34, 24, 24), -4, -4, 24, 24, GraphicsUnit.Pixel, attrs);
                }
            }
        });
        Record(Path.Combine(dir, "rec-compound.emf"), 96, 64, delegate(Graphics g)
        {
            g.SmoothingMode = SmoothingMode.AntiAlias;
            LineJoin[] joins = { LineJoin.Round, LineJoin.Bevel, LineJoin.Miter };
            for (int i = 0; i < 3; i++)
            {
                using (var pen = new Pen(Color.Black, 12))
                {
                    pen.CompoundArray = new float[] { 0, 0.25f, 0.75f, 1 };
                    pen.LineJoin = joins[i];
                    g.DrawRectangle(pen, 10 + 30 * i, 10, 16, 40);
                }
            }
        });
        Record(Path.Combine(dir, "rec-caps.emf"), 96, 64, delegate(Graphics g)
        {
            g.SmoothingMode = SmoothingMode.AntiAlias;
            using (var pen = new Pen(Color.Black, 4))
            {
                pen.CustomEndCap = new AdjustableArrowCap(3, 4, true);
                g.DrawLine(pen, 10, 12, 60, 12);
            }
            using (var pen = new Pen(Color.Black, 4))
            using (var capPath = new GraphicsPath())
            {
                capPath.AddPolygon(new PointF[] { new PointF(-1, -1), new PointF(1, -1), new PointF(0.5f, 2) });
                pen.CustomEndCap = new CustomLineCap(capPath, null);
                g.DrawLine(pen, 10, 32, 60, 32);
            }
            using (var pen = new Pen(Color.Black, 4))
            using (var capPath = new GraphicsPath())
            {
                capPath.AddLines(new PointF[] { new PointF(-1, -1), new PointF(0, 1), new PointF(1, -1) });
                pen.CustomStartCap = new CustomLineCap(null, capPath);
                g.DrawLine(pen, 10, 52, 60, 52);
            }
        });
        Record(Path.Combine(dir, "rec-pathgrad.emf"), 96, 64, delegate(Graphics g)
        {
            using (var path = new GraphicsPath())
            {
                path.AddRectangle(new Rectangle(4, 4, 40, 40));
                using (var b = new PathGradientBrush(path))
                {
                    b.CenterColor = Color.Red;
                    b.SurroundColors = new Color[] { Color.Blue };
                    var blend = new Blend(3);
                    blend.Factors = new float[] { 0, 0.8f, 1 };
                    blend.Positions = new float[] { 0, 0.5f, 1 };
                    b.Blend = blend;
                    g.FillRectangle(b, 4, 4, 40, 40);
                }
            }
            using (var path = new GraphicsPath())
            {
                path.AddRectangle(new Rectangle(52, 4, 40, 40));
                using (var b = new PathGradientBrush(path))
                {
                    var cb = new ColorBlend(3);
                    cb.Colors = new Color[] { Color.Blue, Color.Lime, Color.Red };
                    cb.Positions = new float[] { 0, 0.5f, 1 };
                    b.InterpolationColors = cb;
                    g.FillRectangle(b, 52, 4, 40, 40);
                }
            }
        });
        Record(Path.Combine(dir, "rec-hatch.emf"), 96, 64, delegate(Graphics g)
        {
            for (int s = 0; s <= 52; s++)
            {
                using (var b = new HatchBrush((HatchStyle)s, Color.Black, Color.White))
                {
                    g.FillRectangle(b, (s % 8) * 12, (s / 8) * 9, 8, 8);
                }
            }
        });
        return recordErrors.ToString();
    }

    public static void RenderGdiPlus(string path, int w, int h, string png)
    {
        using (var bmp = new Bitmap(w, h, PixelFormat.Format24bppRgb))
        using (var g = Graphics.FromImage(bmp))
        using (var mf = new Metafile(path))
        {
            g.Clear(Color.White);
            g.DrawImage(mf, new Rectangle(0, 0, w, h));
            bmp.Save(png, ImageFormat.Png);
        }
    }

    // RenderGdi plays an EMF with PlayEnhMetaFile into a white bitmap; it
    // returns false for files that are not EMF.
    public static bool RenderGdi(string path, int w, int h, string png)
    {
        IntPtr hemf = GetEnhMetaFileW(path);
        if (hemf == IntPtr.Zero)
        {
            return false;
        }
        try
        {
            using (var bmp = new Bitmap(w, h, PixelFormat.Format24bppRgb))
            {
                using (var g = Graphics.FromImage(bmp))
                {
                    g.Clear(Color.White);
                    IntPtr hdc = g.GetHdc();
                    var r = new RECT { Left = 0, Top = 0, Right = w, Bottom = h };
                    PlayEnhMetaFile(hdc, hemf, ref r);
                    g.ReleaseHdc(hdc);
                }
                bmp.Save(png, ImageFormat.Png);
            }
        }
        finally
        {
            DeleteEnhMetaFile(hemf);
        }
        return true;
    }
}
