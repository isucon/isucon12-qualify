package isucon12;

import org.springframework.context.annotation.Configuration;
import org.springframework.http.CacheControl;
import org.springframework.web.servlet.config.annotation.InterceptorRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

@Configuration
public class Config implements WebMvcConfigurer {
    @Override
    public void addInterceptors(InterceptorRegistry registry) {
        CacheControlInterceptor cci = new CacheControlInterceptor();
        cci.addCacheMapping(CacheControl.empty().cachePrivate(), "/**");
        registry.addInterceptor(cci).addPathPatterns("/**");
    }
}