# A deliberately large, realistic node_modules image: a monorepo whose apps each
# carry their own node_modules (the shape that makes real repos huge), installed
# in a single RUN so the result is one dominant multi-GB layer sitting on the
# node base layers.
#
# That layer-size distribution is the point. compbench times the *makespan* of a
# whole image, so when one layer holds most of the bytes, jobConcurrency (which
# parallelises across layers) cannot help a codec that is serial within a single
# stream -- only a codec that parallelises inside the layer gains. Small,
# evenly-sized corpora like alpine/golang hide that entirely.
#
# Built size is ~13.7GiB total with a ~13GB dominant layer (~92% of the bytes).
FROM node:22-bookworm

ENV PLAYWRIGHT_BROWSERS_PATH=0 \
    npm_config_audit=false \
    npm_config_fund=false \
    npm_config_loglevel=error

WORKDIR /srv/monorepo

# Replicate the installed workspaces until the tree passes this size.
ARG TARGET_GIB=11

RUN set -eux; \
    mk() { mkdir -p "apps/$1"; printf '{"name":"%s","version":"1.0.0","private":true,"dependencies":{%s}}\n' "$1" "$2" > "apps/$1/package.json"; }; \
    mk next-app        '"next":"14.2.15","react":"18.3.1","react-dom":"18.3.1"'; \
    mk angular-app     '"@angular/core":"18.2.0","@angular/common":"18.2.0","@angular/cli":"18.2.0","rxjs":"7.8.1","zone.js":"0.14.10"'; \
    mk vue-app         '"vue":"3.5.12","vite":"5.4.10","@vitejs/plugin-vue":"5.1.4","pinia":"2.2.4"'; \
    mk nest-api        '"@nestjs/core":"10.4.4","@nestjs/common":"10.4.4","@nestjs/platform-express":"10.4.4","typeorm":"0.3.20"'; \
    mk bundler         '"webpack":"5.95.0","webpack-cli":"5.1.4","babel-loader":"9.2.1","@babel/core":"7.25.8","@babel/preset-env":"7.25.8"'; \
    mk testing         '"jest":"29.7.0","ts-jest":"29.2.5","typescript":"5.6.3","@types/jest":"29.5.13"'; \
    mk e2e-playwright  '"playwright":"1.48.2","@playwright/test":"1.48.2"'; \
    mk e2e-cypress     '"cypress":"13.15.0"'; \
    mk desktop         '"electron":"32.2.0"'; \
    mk cloud           '"@aws-sdk/client-s3":"3.677.0","@aws-sdk/client-dynamodb":"3.677.0","@aws-sdk/client-ec2":"3.677.0","@aws-sdk/client-lambda":"3.677.0"'; \
    mk lint            '"eslint":"9.13.0","prettier":"3.3.3","typescript-eslint":"8.11.0","typescript":"5.6.3"'; \
    mk dataviz         '"three":"0.169.0","d3":"7.9.0","chart.js":"4.4.5","lodash":"4.17.21","moment":"2.30.1"'; \
    for a in apps/*; do \
      echo "=== npm install $a"; \
      ( cd "$a" && for try in 1 2 3; do npm install --no-package-lock && break || sleep 10; done ) || echo "WARN: $a incomplete"; \
    done; \
    ( cd apps/e2e-playwright && npx --yes playwright install chromium firefox webkit ) || echo "WARN: playwright browsers skipped"; \
    target=$(( TARGET_GIB * 1024 * 1024 )); \
    i=0; \
    while [ "$(du -sk . | cut -f1)" -lt "$target" ]; do \
      i=$(( i + 1 )); \
      cp -a apps "workspace-$i"; \
      echo "replica $i -> $(du -sh . | cut -f1)"; \
      [ "$i" -gt 40 ] && break; \
    done; \
    du -sh .; find . -type f | wc -l
