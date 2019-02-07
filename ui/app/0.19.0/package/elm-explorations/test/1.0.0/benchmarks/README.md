# Benchmarks for elm-test

These are some benchmarks of the elm-test library built using the excellent [elm-benchmark](https://github.com/BrianHicks/elm-benchmark).

## How to run

```sh
cd ./benchmarks
elm-make Main.elm
open index.html
```

## How to use
These benchmarks can help get an idea of the performance impact of a change in the elm-test code.
Beware however that a fifty percent performance increase in these benchmarks will most likely not translate to a fifty percent faster tests for users.
In real word scenario's the execution of the test body will have a significant impact on the running time of the test suite, an aspect we're not testing here because it's different for every test suite.
To get a feeling for the impact your change has on actual test run times try running some real test suites with and without your changes.

## Benchmarking complete test suites
These are some examples of test suites that contain a lot of fuzzer tests:
- [elm-benchmark](https://github.com/BrianHicks/elm-benchmark)
- [elm-nonempty-list](https://github.com/mgold/elm-nonempty-list)
- [json-elm-schema](https://github.com/NoRedInk/json-elm-schema)

A tool you can use for benchmarking the suite is [bench](https://github.com/Gabriel439/bench).

To run the tests using your modified code (this only works if your modified version is backwards compatible with the version of elm-test currenlty in use by the test suite):
- In your test suite directories `elm-package.json`:
  - Remove the dependency on `elm-test`.
  - Add dependecies of `elm-test` as dependencies of the test suite itself.
  - Add the path to your changed elm-test src directory to your `source-directories`.
    It will be something like `/<projects-dir>/elm-test/src`.
- Run `elm-test` once to trigger compilation.
- Now run `elm-test` with your benchmarking tool.
