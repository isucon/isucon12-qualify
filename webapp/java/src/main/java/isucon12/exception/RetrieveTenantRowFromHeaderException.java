package isucon12.exception;

public class RetrieveTenantRowFromHeaderException extends Exception {
    private static final long serialVersionUID = 5188397249425697960L;

    public RetrieveTenantRowFromHeaderException(String message) {
        this(message, null);
    }

    public RetrieveTenantRowFromHeaderException(String message, Throwable cause) {
        super(message, cause);
    }
}
