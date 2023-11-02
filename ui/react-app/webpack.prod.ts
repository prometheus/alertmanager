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

// const CompressionPlugin = require("compression-webpack-plugin");
import { Configuration } from 'webpack';
import { merge } from 'webpack-merge';
import { ESBuildMinifyPlugin } from 'esbuild-loader';
import { commonConfig } from './webpack.common';
import path from 'path';

const prodConfig: Configuration = {
  output: {
    path: path.resolve(__dirname, './dist'),
    publicPath: '/react-app/',
  },
  mode: 'production',
  bail: true,
  devtool: 'source-map',
  optimization: {
    // TODO: Could this also be replaced with swc minifier?
    minimizer: [new ESBuildMinifyPlugin({ target: 'es2018' })],
  },
  /*plugins: [
    new CompressionPlugin({
      deleteOriginalAssets: false,
      algorithm: 'gzip',
    })
  ]*/
};

const merged = merge(commonConfig, prodConfig);
export default merged;
