package isucon12.exception;

public class RetrievePlayerException extends Exception {
    private static final long serialVersionUID = -4484327931309498868L;

    public RetrievePlayerException(String message) {
        this(message, null);
    }

    public RetrievePlayerException(String message, Throwable cause) {
        super(message, cause);
    }
}
