package isucon12.exception;

public class BillingReportByCompetitionException extends Exception {
    private static final long serialVersionUID = -6892636779127697465L;

    public BillingReportByCompetitionException(String message) {
        this(message, null);
    }

    public BillingReportByCompetitionException(String message, Throwable cause) {
        super(message, cause);
    }
}
