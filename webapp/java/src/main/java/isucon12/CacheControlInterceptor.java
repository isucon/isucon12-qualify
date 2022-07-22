package isucon12;

import org.springframework.http.CacheControl;
import org.springframework.web.servlet.mvc.WebContentInterceptor;

public class CacheControlInterceptor extends WebContentInterceptor {
    @Override
    public void addCacheMapping(CacheControl cacheControl, String... paths) {
        super.addCacheMapping(cacheControl, paths);
    }
}
