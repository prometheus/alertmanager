FROM node:latest

ENV ELM_FORMAT_VERSION 0.6.0-alpha
ENV ELM_VERSION 0.18

RUN npm install -g elm@0.18.0

RUN wget https://github.com/avh4/elm-format/releases/download/${ELM_FORMAT_VERSION}/elm-format-${ELM_VERSION}-${ELM_FORMAT_VERSION}-linux-x64.tgz -O "elm-format"  && \
    tar -xvzf elm-format && \
    mv elm-format /usr/local/bin/elm-format && \
    chmod +x /usr/local/bin/elm-format