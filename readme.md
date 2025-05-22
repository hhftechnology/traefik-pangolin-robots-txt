# Pangolin Robots.txt Traefik plugin

#### Table of Contents

1. [Description](#description)
2. [Setup](#setup)
3. [Usage](#usage)
4. [Reference](#reference)
5. [Advanced Features](#advanced-features)
6. [Performance and Caching](#performance-and-caching)

## Description

Robots.txt is a middleware plugin for [Traefik](https://traefik.io/) which dynamically creates or enhances the `/robots.txt` file of your website. The plugin supports multiple content sources including [ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt/) rules, local files, custom URLs, and user-defined rules.

**Key Features:**
- **Intelligent Caching**: Reduces external API calls with configurable TTL
- **Multiple Content Sources**: Support for URLs, local files, and custom rules
- **Robust Error Handling**: Fallback mechanisms and retry logic
- **Performance Monitoring**: Optional metrics and detailed logging
- **Content Validation**: Ensures fetched robots.txt content meets basic standards
- **Flexible Configuration**: Extensive customization options for various use cases

## Setup

```yaml
# Static configuration

experimental:
  plugins:
    pangolin-robots-txt:
      moduleName: github.com/hhftechnology/traefik-pangolin-robots-txt
      version: v1.0.0
```

## Usage

### Basic Usage with Custom Rules

```yaml
# Dynamic configuration

http:
  routers:
    my-router:
      rule: host(`localhost`)
      service: service-foo
      entryPoints:
        - web
      middlewares:
        - my-pangolin-robots-txt

  services:
   service-foo:
      loadBalancer:
        servers:
          - url: http://127.0.0.1
  
  middlewares:
    my-pangolin-robots-txt:
      plugin:
        pangolin-robots-txt:
          customRules: |
            User-agent: *
            Disallow: /private/
            Disallow: /admin/
            Allow: /public/
```

### Advanced Usage with AI Robots.txt and Caching

```yaml
middlewares:
  enhanced-robots-txt:
    plugin:
      robots-txt:
        aiRobotsTxt: true
        customRules: |
          User-agent: *
          Disallow: /api/
          Sitemap: https://example.com/sitemap.xml
        cacheTtl: 1800  # Cache for 30 minutes
        enableMetrics: true
        fallbackContent: |
          User-agent: *
          Disallow: /
```

### Using Local File Source

```yaml
middlewares:
  file-based-pangolin-robots-txt:
    plugin:
      pangolin-robots-txt:
        aiRobotsTxt: true
        aiRobotsTxtPath: "/app/config/ai-robots.txt"  # Local file takes precedence over URL
        customRules: |
          User-agent: *
          Allow: /
        overwrite: false
        cacheTtl: 3600  # Cache for 1 hour
```

### Custom AI Robots.txt Source

```yaml
middlewares:
  custom-source-pangolin-robots-txt:
    plugin:
      pangolin-robots-txt:
        aiRobotsTxt: true
        aiRobotsTxtUrl: "https://my-company.com/internal/ai-robots.txt"
        customRules: |
          User-agent: CompanyBot
          Allow: /
        requestTimeout: 15
        maxRetries: 5
```

## Reference

### Configuration Options

| Name              | Type    | Description                                           | Default Value | Example                              |
|-------------------|---------|-------------------------------------------------------|---------------|--------------------------------------|
| `customRules`     | string  | Custom robots.txt rules to append or use exclusively | `""`          | `"\nUser-agent: *\nDisallow: /private/\n"` |
| `overwrite`       | boolean | Replace original robots.txt content completely       | `false`       | `true`                               |
| `aiRobotsTxt`     | boolean | Enable fetching AI robots.txt rules                  | `false`       | `true`                               |
| `lastModified`    | boolean | Preserve Last-Modified headers from backend          | `false`       | `true`                               |

### Advanced Configuration Options

| Name                | Type    | Description                                           | Default Value | Example                              |
|---------------------|---------|-------------------------------------------------------|---------------|--------------------------------------|
| `aiRobotsTxtUrl`    | string  | Custom URL for AI robots.txt source                  | GitHub URL    | `"https://internal.com/robots.txt"`  |
| `aiRobotsTxtPath`   | string  | Local file path for AI robots.txt (overrides URL)   | `""`          | `"/app/config/ai-robots.txt"`        |
| `cacheTtl`          | integer | Cache duration in seconds for external content       | `300`         | `1800` (30 minutes)                  |
| `maxRetries`        | integer | Maximum retry attempts for failed external requests  | `3`           | `5`                                  |
| `requestTimeout`    | integer | HTTP request timeout in seconds                      | `10`          | `30`                                 |
| `fallbackContent`   | string  | Content to use when external sources fail            | `""`          | `"User-agent: *\nDisallow: /"`       |
| `enableMetrics`     | boolean | Enable detailed logging and metrics collection       | `false`       | `true`                               |

## Advanced Features

### Intelligent Caching System

The plugin includes a sophisticated caching mechanism that significantly reduces external API calls and improves performance:

- **Configurable TTL**: Set cache duration based on your needs (from minutes to hours)
- **Source-Aware Caching**: Different cache entries for different sources (URLs vs files)
- **Thread-Safe Operations**: Safe for high-concurrency environments
- **Cache Miss Handling**: Graceful degradation when cache expires or fails

### Multiple Content Sources

The plugin supports various content sources with a clear priority order:

1. **Local Files** (`aiRobotsTxtPath`): Highest priority, ideal for containerized deployments
2. **Custom URLs** (`aiRobotsTxtUrl`): Alternative external sources beyond the default GitHub repository
3. **Default AI Repository**: The standard ai.robots.txt GitHub repository
4. **Custom Rules**: Always included, either appended or as exclusive content

### Error Handling and Resilience

The plugin implements several layers of error handling to ensure reliability:

- **Retry Logic**: Configurable retry attempts with exponential backoff
- **Fallback Content**: Predefined content when all external sources fail
- **Content Validation**: Basic validation to ensure fetched content is well-formed
- **Graceful Degradation**: Service continues even when external dependencies fail

### Performance Monitoring

When `enableMetrics` is enabled, the plugin provides detailed insights:

- **Cache Performance**: Hit/miss ratios and cache effectiveness
- **External Calls**: Number and success rate of external requests
- **Error Tracking**: Detailed error counts and types
- **Response Times**: Performance metrics for optimization

## Performance and Caching

### Cache Strategy

The caching system is designed to balance freshness with performance:

```yaml
# Aggressive caching for stable content
cacheTtl: 3600  # 1 hour

# Moderate caching for dynamic content  
cacheTtl: 900   # 15 minutes

# Minimal caching for frequently updated content
cacheTtl: 300   # 5 minutes
```

### File vs URL Performance

Local files offer significant performance advantages:

- **Instant Access**: No network latency or external dependencies
- **Reliability**: Immune to external service outages
- **Security**: No external network calls required
- **Consistency**: Content remains stable across deployments

### Optimization Tips

For optimal performance in production environments:

1. **Use Local Files**: Deploy AI robots.txt content with your application
2. **Set Appropriate Cache TTL**: Balance freshness needs with performance requirements
3. **Enable Metrics**: Monitor cache effectiveness and adjust configuration accordingly
4. **Configure Fallback Content**: Provide meaningful defaults for error scenarios
5. **Adjust Timeouts**: Set realistic timeout values based on your network conditions

### Example Production Configuration

```yaml
middlewares:
  production-pangolin-robots-txt:
    plugin:
      pangolin-robots-txt:
        # Use local file for reliability
        aiRobotsTxt: true
        aiRobotsTxtPath: "/app/robots/ai-robots.txt"
        
        # Custom rules for your application
        customRules: |
          User-agent: *
          Disallow: /admin/
          Disallow: /api/v1/internal/
          Allow: /api/v1/public/
          Sitemap: https://example.com/sitemap.xml
          
        # Production settings
        cacheTtl: 1800        # 30-minute cache
        requestTimeout: 20    # Generous timeout
        maxRetries: 3         # Reasonable retry attempts
        enableMetrics: true   # Monitor performance
        
        # Fallback for emergencies
        fallbackContent: |
          User-agent: *
          Disallow: /
```
