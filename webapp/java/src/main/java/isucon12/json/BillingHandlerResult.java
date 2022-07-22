package isucon12.json;

import java.util.List;

public class BillingHandlerResult {
    private List<BillingReport> reports;

    public BillingHandlerResult(List<BillingReport> reports) {
        super();
        this.reports = reports;
    }

    public List<BillingReport> getReports() {
        return reports;
    }

    public void setReports(List<BillingReport> reports) {
        this.reports = reports;
    }
}
