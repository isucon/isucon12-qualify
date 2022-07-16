package isucon12.exception;

public class DatabaseException extends Exception {
    private static final long serialVersionUID = -8973155754580143503L;

    public DatabaseException(String message) {
        this(message, null);
    }

    public DatabaseException(String message, Throwable cause) {
        super(message, cause);
    }
}
