# Randomness

Need to generate random numbers? How about random game boards? Or random positions in 3D space? This is the package for you!


## Example

This package will help you build a random `Generator` like this:

```elm
import Random

probability : Random.Generator Float
probability =
  Random.float 0 1

roll : Random.Generator Int
roll =
  Random.int 1 6

usuallyTrue : Random.Generator Bool
usuallyTrue =
  Random.weighted (80, True) [ (20, False) ]
```

In each of these defines _how_ to generate random values. The most interesting case is `usuallyTrue` which generates `True` 80% of the time and `False` 20% of the time!

Now look at this [working example](https://elm-lang.org/examples/random) to see a `Generator` used in an application.


## Mindset Shift

If you are coming from JavaScript, this package is usually quite surprising at first. Why not just call `Math.random()` and get random floats whenever you want? Well, all Elm functions have this “same input, same output” guarantee. That is part of what makes Elm so reliable and easy to test! But if we could generate random values anytime we want, we would have to throw that guarantee out.

So instead, we create a `Generator` and hand it to the Elm runtime system to do the dirty work of generating values. We get to keep our guarantees _and_ we get random values. Great! And once people become familiar with generators, they often report that it is _easier_ than the traditional imperative APIs for most cases. For example, jump to the docs for [`Random.map4`](Random#map4) for an example of generating random [quadtrees](https://en.wikipedia.org/wiki/Quadtree) and think about what it would look like to do that in JavaScript!

Point is, this library takes some learning, but we really think it is worth it. So hang in there, and do not hesitate to ask for help on [Slack](https://elmlang.herokuapp.com/) or [Discourse](https://discourse.elm-lang.org/)!


## Future Plans

There are a ton of useful helper functions in the [`elm-community/random-extra`][extra] package. Do you need random `String` values? Random dictionaries? Etc.

We will probably do an API review and merge the results into this package someday. Not sure when, but it would be kind of nice to have it all in one place. But in the meantime, just do `elm install elm-community/random-extra` if you need stuff from there!

[extra]: /packages/elm-community/random-extra/latest
