package isucon12.exception;

public class FileLockException extends Exception {
    private static final long serialVersionUID = -2780901255888988559L;

    public FileLockException(String message) {
        this(message, null);
    }

    public FileLockException(String message, Throwable cause) {
        super(message, cause);
    }
}
