FROM node:22-bookworm

ENV NPM_CONFIG_PREFIX=/home/node/.npm-global
ENV PATH=$PATH:/home/node/.npm-global/bin

RUN mkdir -p $NPM_CONFIG_PREFIX; yarn global add \
  elm@0.19.1 \
  elm-format@0.8.7 \
  elm-test@0.19.1-revision6 \
  uglify-js@3.13.4 \
  elm-review@2.5.0
