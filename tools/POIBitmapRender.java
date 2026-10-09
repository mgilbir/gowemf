// Execution-only raster oracle adapter using public POI and JDK APIs.
import java.awt.*;
import java.awt.geom.*;
import java.awt.image.*;
import java.io.*;
import java.nio.file.*;
import javax.imageio.ImageIO;
import org.apache.poi.hwmf.usermodel.HwmfPicture;

public class POIBitmapRender {
    public static void main(String[] args) throws Exception {
        if (args.length != 2) throw new IllegalArgumentException("input.wmf output.png");
        Path input=Paths.get(args[0]);
        if (Files.size(input)>1024*1024) throw new IOException("input budget");
        BufferedImage image=new BufferedImage(64,32,BufferedImage.TYPE_INT_ARGB);
        Graphics2D graphics=image.createGraphics();
        graphics.setColor(Color.WHITE); graphics.fillRect(0,0,64,32);
        try(InputStream in=Files.newInputStream(input)) {
            new HwmfPicture(in).draw(graphics,new Rectangle2D.Double(0,0,64,32));
        } finally { graphics.dispose(); }
        if (!ImageIO.write(image,"png",new File(args[1]))) throw new IOException("PNG encoder unavailable");
    }
}
