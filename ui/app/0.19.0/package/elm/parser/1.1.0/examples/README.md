# Run the Examples

To try these examples out locally, you can run the following terminal commands:

```bash
git clone https://github.com/elm/parser.git
cd parser/examples
elm reactor
```

After that, go to [`http://localhost:8000`](http://localhost:8000) and click on
the example you want to see.


## Exercises

- Have a user input feed into the `Math` parser. Show people the results live.
- Expand the `Math` parser to cover `-` and `/` as well.
- Handle more escape characters in `DoubleQuotedString`. Maybe hexidecimal
escapes like `\x41` and `\x0A` that are possible in JavaScript.