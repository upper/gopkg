# vanity package names using custom domains

Use `vanity` to use custom domains on Go packages and redirect them to public
Git repositories:

```
import (
  "example.org/coolpkg"
)
```

## A simple example

Let's see the available parameters:

```
vanity -h
Usage of ./gopkg:
  -addr string
        Serve HTTP at given address (default ":8080")
  -repo-root string
        Git repository root URL (e.g.: https://github.com/upper).
  -vanity-root string
        Vanity root URL (e.g.: https://upper.io).
```

Now run `vanity` on localhost:

```
vanity -addr localhost:8082 -repo-root https://github.com/golang \
-vanity-root http://localhost:8082`
```

Using a different terminal session, try to `go get` a package from
`localhost:8082`:

```
go get -insecure -v localhost:8082/example/hello
```

`vanity` will tell `go get` to keep the import path and pull the source from a
different place.

Output should be similar to:

```
Fetching https://localhost:8082/example/hello?go-get=1
https fetch failed.
Fetching http://localhost:8082/example/hello?go-get=1
Parsing meta tags from http://localhost:8082/example/hello?go-get=1 (status code 200)
get "localhost:8082/example/hello": found meta tag main.metaImport{Prefix:"localhost:8082/example", VCS:"git", RepoRoot:"http://localhost:8082/example"} at http://localhost:8082/example/hello?go-get=1
get "localhost:8082/example/hello": verifying non-authoritative meta tag
Fetching https://localhost:8082/example?go-get=1
https fetch failed.
Fetching http://localhost:8082/example?go-get=1
Parsing meta tags from http://localhost:8082/example?go-get=1 (status code 200)
localhost:8082/example (download)
localhost:8082/example/hello
```

At the end of the `go get` program, you should have the example `hello` package
on `$GOPATH/src/localhost:8082/example/hello` and in your `$GOPATH/bin`
directory as well:

```go
hello
Hello, Go examples!
```

### Versioning support

`vanity` also comes with versioning support from the original
[http://gopkg.in](http://gopkg.in) with no extra cost. For instance, the
following import

```go
go get -v example.org/coolpkg.v1
```

automatically redirects to the `v1` branch on the github.com/username/coolpkg
repo, and it can also be imported that way:

```go
import (
  "example.org/coolpkg.v1"
)
```

Oh, and `vanity` is not tied to GitHub at all, you can use any public git
repository with https support:

```
vanity -addr :80 -repo-root https://othergitsite.com/username -vanity-root https://example.org
```

## Deploy

It is not recommended to run `vanity` directly, as `vanity` does not have a
nice welcome page nor instructions for humans, `vanity` only expects only
communication from `go get`, if any other kind of request is received it will
return a `404` error.

But fear not, you can run vanity with `nginx` easily by using that fact:

```
vanity -addr 127.0.0.1:9192 -repo-root https://github.com/upper \
-vanity-root https://upper.io
```

This is the site configuration for nginx:

```
location / {
  # First try with vanity.
  try_files =404 @vanity;
}

location @vanity {
  proxy_set_header X-Real-IP  $remote_addr;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

  proxy_pass http://127.0.0.1:9192;
  proxy_intercept_errors on;
  recursive_error_pages on;

  set $pass 0;
  if ($arg_go-get = 1) {
    set $pass 1;
  }
  if ($request_uri ~ git-upload-pack) {
    set $pass 1;
  }
  if ($pass = 0) {
    return 404;
  }

  error_page 404 = @real_location;
}

location @real_location {
  # Fallback location for when vanity returns 404.
  ...
}
```

## License

### gopkg.in

This project was based on [gopkg.in](http://labix.org/gopkg.in) by [Gustavo
Niemeyer](http://labix.org/):

```
gopkg.in - versioned URLs for Go packages

Copyright (c) 2014 - Gustavo Niemeyer <gustavo@niemeyer.net>

All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```
