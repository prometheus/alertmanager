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

import { Configuration } from 'webpack';
import { merge } from 'webpack-merge';
import { ESBuildMinifyPlugin } from 'esbuild-loader';
import { commonConfig } from './webpack.common';

const prodConfig: Configuration = {
  mode: 'production',
  bail: true,
  devtool: 'source-map',
  optimization: {
    // TODO: Could this also be replaced with swc minifier?
    minimizer: [new ESBuildMinifyPlugin({ target: 'es2018' })],
  },
};

const merged = merge(commonConfig, prodConfig);
export default merged;
