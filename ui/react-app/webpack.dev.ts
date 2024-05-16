// Copyright 2023 The Perses Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import fs from 'fs';
import { Configuration } from 'webpack';
import { Configuration as DevServerConfig, ServerConfiguration } from 'webpack-dev-server';
import { merge } from 'webpack-merge';
import { commonConfig } from './webpack.common';
// eslint-disable-next-line @typescript-eslint/no-var-requires
require('dotenv-defaults').config();

declare module 'webpack' {
  interface Configuration {
    devServer?: DevServerConfig | undefined;
  }
}

// Get dev server HTTP options (note: HTTP2 is not currently supported by webpack since we're on Node 16)
function getServerConfig(): ServerConfiguration | undefined {
  // Just use regular HTTP by default
  if (process.env.HTTPS !== 'true') {
    return undefined;
  }

  // Support the same HTTPS options as Creact React App if HTTPS is set
  if (process.env.SSL_KEY_FILE === undefined || process.env.SSL_CRT_FILE === undefined) {
    // Use the default self-signed cert
    return { type: 'https' };
  }

  // Use a custom cert
  return {
    type: 'https',
    options: {
      key: fs.readFileSync(process.env.SSL_KEY_FILE),
      cert: fs.readFileSync(process.env.SSL_CRT_FILE),
    },
  };
}

// Webpack configuration in dev
const devConfig: Configuration = {
  mode: 'development',
  devtool: 'cheap-module-source-map',

  output: {
    pathinfo: true,
  },

  watchOptions: {
    aggregateTimeout: 300,
  },

  devServer: {
    port: parseInt(process.env.PORT ?? '3000'),
    open: true,
    server: getServerConfig(),
    historyApiFallback: true,
    allowedHosts: 'all',
    proxy: {
      '/api': 'http://localhost:9093',
    },
  },
  cache: true,
};

const merged = merge(commonConfig, devConfig);
export default merged;
