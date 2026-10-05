// Execution-only oracle adapter, written against POI's public API. No POI
// implementation code is included here. Use -Xmx256m and an external timeout.
import java.io.*;
import java.nio.file.*;
import org.apache.poi.common.usermodel.GenericRecord;
import org.apache.poi.hwmf.usermodel.HwmfPicture;
import org.apache.poi.hwmf.record.HwmfRecord;
import org.apache.poi.hemf.usermodel.HemfPicture;
import org.apache.poi.hemf.record.emf.HemfRecord;
import org.apache.poi.hemf.record.emf.HemfComment.EmfComment;
import org.apache.poi.hemf.record.emf.HemfComment.EmfCommentDataPlus;
import org.apache.poi.hemf.record.emfplus.HemfPlusRecord;
import org.apache.poi.util.GenericRecordJsonWriter;

public class POIRecordDump {
    static int remaining = Integer.MAX_VALUE;
    static void emit(String format, long type, GenericRecord record) {
        if (remaining-- <= 0) return;
        System.out.println("{\"format\":\"" + format + "\",\"type\":" + type
            + ",\"body\":" + GenericRecordJsonWriter.marshal(record, false) + "}");
    }
    public static void main(String[] args) throws Exception {
        if (args.length < 1 || args.length > 2) throw new IllegalArgumentException("input [display-limit]");
        if (args.length == 2) remaining = Integer.parseInt(args[1]);
        Path p = Paths.get(args[0]);
        if (Files.size(p) > 64L * 1024 * 1024) throw new IOException("input limit");
        try (InputStream in = Files.newInputStream(p)) {
            if (p.toString().toLowerCase(java.util.Locale.ROOT).endsWith(".wmf")) {
                for (HwmfRecord r : new HwmfPicture(in).getRecords()) emit("WMF", r.getWmfRecordType().id, r);
            } else {
                for (HemfRecord r : new HemfPicture(in).getRecords()) {
                    emit("EMF", r.getEmfRecordType().id, r);
                    if (r instanceof EmfComment) {
                        Object data = ((EmfComment)r).getCommentData();
                        if (data instanceof EmfCommentDataPlus) {
                            for (HemfPlusRecord plus : ((EmfCommentDataPlus)data).getRecords()) {
                                long id = plus.getEmfPlusRecordType().getClass().getField("id").getLong(plus.getEmfPlusRecordType());
                                emit("EMF+", id, plus);
                            }
                        }
                    }
                }
            }
        }
    }
}
