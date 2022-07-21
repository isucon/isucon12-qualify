package isucon12.exception;

public class AppException extends Exception {
    private static final long serialVersionUID = 5708315331847599556L;

    public AppException(String message) {
        this(message, null);
    }

    public AppException(String message, Throwable cause) {
        super(message, cause);
    }
}
