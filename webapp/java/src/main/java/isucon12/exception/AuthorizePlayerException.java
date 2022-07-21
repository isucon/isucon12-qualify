package isucon12.exception;

import org.springframework.http.HttpStatus;

public class AuthorizePlayerException extends Exception {
    private static final long serialVersionUID = -8702478412996926206L;
    private final HttpStatus httpStatus;

    public AuthorizePlayerException(HttpStatus httpStatus, String message) {
        this(httpStatus, message, null);
    }

    public AuthorizePlayerException(HttpStatus httpStatus, String message, Throwable cause) {
        super(message, cause);
        this.httpStatus = httpStatus;
    }

    public HttpStatus getHttpStatus() {
        return httpStatus;
    }
}
