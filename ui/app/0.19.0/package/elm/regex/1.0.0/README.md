# Regex in Elm

**Generally speaking, it will be easier and nicer to use a parsing library like [`elm/parser`][elm] instead of this.**

[elm]: https://package.elm-lang.org/packages/elm/parser/latest

That said, sometimes you may want the kind of regular expressions that appear in JavaScript. Maybe you found some regex on StackOverflow and just want to place it in your code directly. This library supports that scenario.



## Future Plans

I hope that _other_ packages will spring up for common parsing tasks, making `regex` less and less useful.

So instead of searching Stack Overflow for "email regex" we could have a well-tested package for validating emails. Instead of searching Stack Overflow for "phone numbers" we could have a well-tested package for validating phone numbers that gathered a bunch of helpful information on handling international numbers. Etc.

And as the community handles more and more cases in an _excellent_ way, I hope a day will come when no one wants the `regex` package anymore.


<br>


## Historical Notes

I want to draw a distinction between **regular expressions** and **regex**. These are related, but not the same. I think understanding the distinction helps motivate why I recommend against using this package.


### Regular Expressions

In theoritical computer science, the idea of a “regular expression” is a simple expression that matches a set of strings. For example:

- `a` matches  `"a"`
- `ab` matches  `"ab"`
- `ab*` matches  `"a"`, `"ab"`, `"abb"`, `"abbb"`, `"abbbb"`, etc.
- `(ab)*` matches  `""`, `"ab"`, `"abab"`, `"ababab"`, `"abababab"`, etc.
- `a|b` matches `"a"` and `"b"`
- `a|(bb)*` matches `"a"`, `""`, `"bb"`, `"bbbb"`, `"bbbb"`, `"bbbbbb"`, etc.

So you basically have `*` to repeat, parentheses for grouping, and `|` for providing alternatives. That is it! A simple syntax that can describe a bunch of different things.

It also has quite beautiful relationships to finite automata, context-free grammers, turing machines, etc. If you are into this sort of thing, I highly recommend [Introduction to the Theory of Computation](https://math.mit.edu/~sipser/book.html) by Michael Sipser!


### Regex

So people came up with that simple thing in computer science, and on its surface, it looks like a good way to match email addresses, phone numbers, etc. But regular expressions only match or not. How can we _extract_ information from the string as well? Well, this is how regex was born.

A bunch of extensions were added to the root idea, significantly complicating the syntax and behavior. For example, instead of using parentheses just for grouping, parentheses also extract information. But wait, how does `(a|b)*` work if we are extracting everything inside the parens? What should be extracted from matching strings like `"aabb"` or `"aba"` now?

So lots of things like that were added, and the result is called “regex” and it appears in a bunch of common programming languages like Perl, Python, and JavaScript.


### Reflections

The regex idea has become quite influential. It is “good enough” for a lot of cases, but it is also quite confusing and difficult to use reliably. If you look around Stack Overflow, you will find tons of questions like "how do I parse an email address?" and many folks just copy/paste the answers without really reading or understanding the regex thoroughly. Does the regex really work? What exactly do you want to allow and disallow?

The root issue is that regular expressions were not _meant_ to parse everything. For example, regular expressions are unable to describe sets of strings with balanced parentheses, so no regular expression can describe the set of `"()"`, `"(())"`, `"((()))"`, etc. (That means [they cannot parse matching HTML tags](https://stackoverflow.com/a/1732454) either!) But you _can_ do that with context-free grammars! With one really elegant addition! So the limitations of regular expressions are actually their whole point. They are _supposed_ to be simple to show why other formulations can express more.

So this is why I recommend the [`elm/parser`][elm] package over this one. It _is_ meant to parse everything, and in a way that works really nice with Elm.
