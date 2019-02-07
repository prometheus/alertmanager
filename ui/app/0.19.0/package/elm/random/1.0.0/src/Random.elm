effect module Random where { command = MyCmd } exposing
  ( Generator, Seed
  , int, float, uniform, weighted, constant
  , list, pair
  , map, map2, map3, map4, map5
  , andThen, lazy
  , minInt, maxInt
  , generate
  , step, initialSeed, independentSeed
  )

{-| This library helps you generate pseudo-random values.

It is an implementation of [Permuted Congruential Generators][pcg]
by M. E. O'Neil. It is not cryptographically secure.

[extra]: /packages/elm-community/random-extra/latest
[pcg]: http://www.pcg-random.org/


# Generators
@docs Generator, generate

# Primitives
@docs int, float, uniform, weighted, constant

# Data Structures
@docs pair, list

# Mapping
@docs map, map2, map3, map4, map5

# Fancy Stuff
@docs andThen, lazy

# Constants
@docs maxInt, minInt

# Generate Values Manually
@docs Seed, step, initialSeed, independentSeed

-}

import Basics exposing (..)
import Bitwise
import List exposing ((::))
import Platform
import Platform.Cmd exposing (Cmd)
import Task exposing (Task)
import Time



-- PRIMITIVE GENERATORS


{-| Generate 32-bit integers in a given range.

    import Random

    singleDigit : Random.Generator Int
    singleDigit =
        Random.int 0 9

    closeToZero : Random.Generator Int
    closeToZero =
        Random.int -5 5

    anyInt : Random.Generator Int
    anyInt =
        Random.int Random.minInt Random.maxInt

This generator *can* produce values outside of the range [[`minInt`](#minInt),
[`maxInt`](#maxInt)] but sufficient randomness is not guaranteed.
-}
int : Int -> Int -> Generator Int
int a b =
    Generator
        (\seed0 ->
            let
                ( lo, hi ) =
                    if a < b then
                        ( a, b )
                    else
                        ( b, a )

                range =
                    hi - lo + 1
            in
                -- fast path for power of 2
                if (Bitwise.and (range - 1) range) == 0 then
                    ( (Bitwise.shiftRightZfBy 0 (Bitwise.and (range - 1) (peel seed0))) + lo, next seed0 )
                else
                    let
                        threshhold =
                            -- essentially: period % max
                            Bitwise.shiftRightZfBy 0 (remainderBy range (Bitwise.shiftRightZfBy 0 -range))

                        accountForBias : Seed -> ( Int, Seed )
                        accountForBias seed =
                            let
                                x =
                                    peel seed

                                seedN =
                                    next seed
                            in
                                if x < threshhold then
                                    -- in practice this recurses almost never
                                    accountForBias seedN
                                else
                                    ( remainderBy range x + lo, seedN )
                    in
                        accountForBias seed0
        )


{-| The underlying algorithm works well in a specific range of integers.
It can generate values outside of that range, but they are “not as random”.

The `maxInt` that works well is `2147483647`.
-}
maxInt : Int
maxInt =
  2147483647


{-| The underlying algorithm works well in a specific range of integers.
It can generate values outside of that range, but they are “not as random”.

The `minInt` that works well is `-2147483648`.
-}
minInt : Int
minInt =
  -2147483648


{-| Generate floats in a given range.

    import Random

    probability : Random.Generator Float
    probability =
        Random.float 0 1

The `probability` generator will produce values between zero and one with
a uniform distribution. Say it produces a value `p`. We can then check if
`p < 0.4` if we want something to happen 40% of the time.

This becomes very powerful when paired with functions like [`map`](#map) and
[`andThen`](#andThen). Rather than dealing with twenty random float messages
in your `update`, you can build up sophisticated logic in the `Generator`
itself!
-}
float : Float -> Float -> Generator Float
float a b =
    Generator (\seed0 ->
        let
            -- Get 64 bits of randomness
            seed1 =
                next seed0

            n0 =
                peel seed0

            n1 =
                peel seed1

            -- Get a uniformly distributed IEEE-754 double between 0.0 and 1.0
            hi =
                toFloat (Bitwise.and 0x03FFFFFF n0) * 1.0

            lo =
                toFloat (Bitwise.and 0x07FFFFFF n1) * 1.0

            val =
                -- These magic constants are 2^27 and 2^53
                ((hi * 134217728.0) + lo) / 9007199254740992.0

            -- Scale it into our range
            range =
                abs (b - a)

            scaled =
                val * range + a
        in
            ( scaled, next seed1 )
        )


{-| Generate the same value every time.

    import Random

    alwaysFour : Random.Generator Int
    alwaysFour =
        Random.constant 4

Think of it as picking from a hat with only one thing in it. It is weird,
but it can be useful with [`elm-community/random-extra`][extra] which has
tons of nice helpers.

[extra]: /packages/elm-community/random-extra/latest
-}
constant : a -> Generator a
constant value =
  Generator (\seed -> (value, seed))



-- DATA STRUCTURES


{-| Generate a pair of random values. A common use of this might be to generate
a point in a certain 2D space:

    import Random

    randomPoint : Random.Generator (Float, Float)
    randomPoint =
        Random.pair (Random.float -200 200) (Random.float -100 100)

Maybe you are doing an animation with SVG and want to randomly generate some
entities?
-}
pair : Generator a -> Generator b -> Generator (a,b)
pair genA genB =
  map2 (\a b -> (a,b)) genA genB


{-| Generate a list of random values.

    import Random

    tenFractions : Random.Generator (List Float)
    tenFractions =
        Random.list 10 (Random.float 0 1)

    fiveGrades : Random.Generator (List Int)
    fiveGrades =
        Random.list 5 (int 0 100)

If you want to generate a list with a random length, you need to use
[`andThen`](#andThen) like this:

    fiveToTenDigits : Random.Generator (List Int)
    fiveToTenDigits =
        Random.int 5 10
          |> Random.andThen (\len -> Random.list len (Random.int 0 9))

This generator gets a random integer between five and ten **and then**
uses that to generate a random list of digits.
-}
list : Int -> Generator a -> Generator (List a)
list n (Generator gen) =
  Generator (\seed ->
    listHelp [] n gen seed
  )


listHelp : List a -> Int -> (Seed -> (a,Seed)) -> Seed -> (List a, Seed)
listHelp revList n gen seed =
  if n < 1 then
    (revList, seed)

  else
    let
      (value, newSeed) =
        gen seed
    in
      listHelp (value :: revList) (n-1) gen newSeed



-- ENUMERATIONS


{-| Generate values with equal probability. Say we want a random suit for some
cards:

    import Random

    type Suit = Diamond | Club | Heart | Spade

    suit : Random.Generator Suit
    suit =
      Random.uniform Diamond [ Club, Heart, Spade ]

That generator produces all `Suit` values with equal probability, 25% each.

**Note:** Why not have `uniform : List a -> Generator a` as the API? It looks
a little prettier in code, but it leads to an awkward question. What do you do
with `uniform []`? How can it produce an `Int` or `Float`? The current API
guarantees that we always have *at least* one value, so we never run into that
question!
-}
uniform : a -> List a -> Generator a
uniform value valueList =
  weighted (addOne value) (List.map addOne valueList)


addOne : a -> (Float, a)
addOne value =
  ( 1, value )


{-| Generate values with a _weighted_ probability. Say we want to simulate a
[loaded die](https://en.wikipedia.org/wiki/Dice#Loaded_dice) that lands
on ⚄ and ⚅ more often than the other faces:

    import Random

    type Face = One | Two | Three | Four | Five | Six

    roll : Random.Generator Face
    roll =
      Random.weighted
        (10, One)
        [ (10, Two)
        , (10, Three)
        , (10, Four)
        , (20, Five)
        , (40, Six)
        ]

So there is a 40% chance of getting `Six`, a 20% chance of getting `Five`, and
then a 10% chance for each of the remaining faces.

**Note:** I made the weights add up to 100, but that is not necessary. I always
add up your weights into a `total`, and from there, the probablity of each case
is `weight / total`. Negative weights do not really make sense, so I just flip
them to be positive.
-}
weighted : (Float, a) -> List (Float, a) -> Generator a
weighted first others =
  let
    normalize (weight, _) =
      abs weight

    total =
      normalize first + List.sum (List.map normalize others)
  in
  map (getByWeight first others) (float 0 total)


getByWeight : (Float, a) -> List (Float, a) -> Float -> a
getByWeight (weight, value) others countdown =
  case others of
    [] ->
      value

    second :: otherOthers ->
      if countdown <= abs weight then
        value
      else
        getByWeight second otherOthers (countdown - abs weight)



-- CUSTOM GENERATORS


{-| Transform the values produced by a generator. For example, we can
generate random boolean values:

    import Random

    bool : Random.Generator Bool
    bool =
      Random.map (\n -> n < 20) (Random.int 1 100)

The `bool` generator first picks a number between 1 and 100. From there
it checks if the number is less than twenty. So the resulting `Bool` will
be `True` about 20% of the time.

You could also do this for lower case ASCII letters:

    letter : Random.Generator Char
    letter =
      Random.map (\n -> Char.fromCode (n + 97)) (Random.int 0 25)

The `letter` generator first picks a number between 0 and 25 inclusive.
It then uses `Char.fromCode` to turn [ASCII codes][ascii] into `Char` values.

**Note:** Instead of making these yourself, always check if the
[`random-extra`][extra] package has what you need!

[ascii]: https://en.wikipedia.org/wiki/ASCII#Printable_characters
[extra]: /packages/elm-community/random-extra/latest
-}
map : (a -> b) -> Generator a -> Generator b
map func (Generator genA) =
  Generator (\seed0 ->
    let
      (a, seed1) = genA seed0
    in
      (func a, seed1)
  )


{-| Combine two generators. Maybe we have a space invaders game and want to
generate enemy ships with a random location:

    import Random

    type alias Enemy
      { health : Float
      , rotation : Float
      , x : Float
      , y : Float
      }

    enemy : Random.Generator Enemy
    enemy =
      Random.map2
        (\x y -> Enemy 100 0 x y)
        (Random.float 0 100)
        (Random.float 0 100)

Now whenever we run the `enemy` generator we get an enemy with full health,
no rotation, and a random position! Now say we want to start with between
five and ten enemies on screen:

    initialEnemies : Random.Generator (List Enemy)
    initialEnemies =
      Random.int 5 10
        |> Random.andThen (\num -> Random.list num enemy)

We will generate a number between five and ten, **and then** generate
that number of enimies!

**Note:** Snapping generators together like this is very important! Always
start by with generators for each `type` you need, and then snap them
together.
-}
map2 : (a -> b -> c) -> Generator a -> Generator b -> Generator c
map2 func (Generator genA) (Generator genB) =
  Generator (\seed0 ->
    let
      (a, seed1) = genA seed0
      (b, seed2) = genB seed1
    in
      (func a b, seed2)
  )


{-| Combine three generators. Maybe you want to make a simple slot machine?

    import Random

    type alias Spin =
      { one : Symbol
      , two : Symbol
      , three : Symbol
      }

    type Symbol = Cherry | Seven | Bar | Grapes

    spin : Random.Generator Spin
    spin =
      Random.map3 Spin symbol symbol symbol

    symbol : Random.Generator Symbol
    symbol =
      Random.uniform Cherry [ Seven, Bar, Grapes ]

**Note:** Always start with the types. Make a generator for each thing you need
and then put them all together with one of these `map` functions.
-}
map3 : (a -> b -> c -> d) -> Generator a -> Generator b -> Generator c -> Generator d
map3 func (Generator genA) (Generator genB) (Generator genC) =
  Generator (\seed0 ->
    let
      (a, seed1) = genA seed0
      (b, seed2) = genB seed1
      (c, seed3) = genC seed2
    in
      (func a b c, seed3)
  )


{-| Combine four generators.

Say you are making game and want to place enemies or terrain randomly. You
_could_ generate a [quadtree](https://en.wikipedia.org/wiki/Quadtree)!

    import Random

    type QuadTree a
      = Empty
      | Leaf a
      | Node (QuadTree a) (QuadTree a) (QuadTree a) (QuadTree a)

    quadTree : Random.Generator a -> Random.Generator (QuadTree a)
    quadTree leafGen =
      let
        subQuads =
          Random.lazy (\_ -> quadTree leafGen)
      in
      Random.andThen identity <|
        Random.uniform
          (Random.constant Empty)
          [ Random.map Leaf leafGen
          , Random.map4 Node subQuad subQuad subQuad subQuad
          ]

We start by creating a `QuadTree` type where each quadrant is either `Empty`, a
`Leaf` containing something interesting, or a `Node` with four sub-quadrants.

Next the `quadTree` definition describes how to generate these values. A third
of a time you get an `Empty` tree. A third of the time you get a `Leaf` with
some interesting value. And a third of the time you get a `Node` full of more
`QuadTree` values. How are those subtrees generated though? Well, we use our
`quadTree` generator!

**Exercises:** Can `quadTree` generate infinite `QuadTree` values? Is there
some way to limit the depth of the `QuadTree`? Can you render the `QuadTree`
to HTML using absolute positions and fractional dimensions? Can you render
the `QuadTree` to SVG?

**Note:** Check out the docs for [`lazy`](#lazy) to learn why that is needed
to define a recursive `Generator` like this one.
-}
map4 : (a -> b -> c -> d -> e) -> Generator a -> Generator b -> Generator c -> Generator d -> Generator e
map4 func (Generator genA) (Generator genB) (Generator genC) (Generator genD) =
  Generator (\seed0 ->
    let
      (a, seed1) = genA seed0
      (b, seed2) = genB seed1
      (c, seed3) = genC seed2
      (d, seed4) = genD seed3
    in
      (func a b c d, seed4)
  )


{-| Combine five generators.

If you need to combine more things, you can always do it by chaining
[`andThen`](#andThen). There are also some additional helpers for this
in [`elm-community/random-extra`][extra].

[extra]: /packages/elm-community/random-extra/latest
-}
map5 : (a -> b -> c -> d -> e -> f) -> Generator a -> Generator b -> Generator c -> Generator d -> Generator e -> Generator f
map5 func (Generator genA) (Generator genB) (Generator genC) (Generator genD) (Generator genE) =
  Generator (\seed0 ->
    let
      (a, seed1) = genA seed0
      (b, seed2) = genB seed1
      (c, seed3) = genC seed2
      (d, seed4) = genD seed3
      (e, seed5) = genE seed4
    in
      (func a b c d e, seed5)
  )


{-| Generate fancy random values.

We have seen examples of how `andThen` can be used to generate variable length
lists in the [`list`](#list) and [`map2`](#map2) docs. We saw how it could help
generate a quadtree in the [`map4`](#map4) docs.

Anything you could ever want can be defined using this operator! As one last
example, here is how you can define `map` using `andThen`:

    import Random

    map : (a -> b) -> Random.Generator a -> Random.Generator b
    map func generator =
      generator
        |> Random.andThen (\value -> Random.constant (func value))

The `andThen` function gets used a lot in [`elm-community/random-extra`][extra],
so it may be helpful to look through the implementation there for more examples.

[extra]: /packages/elm-community/random-extra/latest
-}
andThen : (a -> Generator b) -> Generator a -> Generator b
andThen callback (Generator genA) =
  Generator (\seed ->
    let
      (result, newSeed) = genA seed
      (Generator genB) = callback result
    in
      genB newSeed
  )


{-| Helper for defining self-recursive generators. Say we want to generate a
random number of probabilities:

    import Random

    probabilities : Random.Generator (List Float)
    probabilities =
      Random.andThen identity <|
        Random.uniform
          [ Random.constant []
          , Random.map2 (::)
              (Random.float 0 1)
              (Random.lazy (\_ -> probabilities))
          ]

In 50% of cases we end the list. In 50% of cases we generate a probability and
add it onto a random number of probabilities. The `lazy` call is crucial
because it means we do not unroll the generator unless we need to.

This is a pretty subtle issue, so I recommend reading more about it
[here](https://elm-lang.org/hints/0.19.0/bad-recursion)!

**Note:** You can delay evaluation with `andThen` too. The thing that matters
is that you have a function call that delays the creation of the generator!
-}
lazy : (() -> Generator a) -> Generator a
lazy callback =
  Generator (\seed ->
    let
      (Generator gen) = callback ()
    in
      gen seed
  )


-- IMPLEMENTATION

{- Explanation of the PCG algorithm

    This is a special variation (dubbed RXS-M-SH) that produces 32
    bits of output by keeping 32 bits of state. There is one function
    (next) to derive the following state and another (peel) to obtain 32
    psuedo-random bits from the current state.

    Getting the next state is easy: multiply by a magic factor, modulus by 2^32,
    and then add an increment. If two seeds have different increments,
    their random numbers from the two seeds will never match up; they are
    completely independent. This is very helpful for isolated components or
    multithreading, and elm-test relies on this feature.

    Transforming a seed into 32 random bits is more complicated, but
    essentially you use the "most random" bits to pick some way of scrambling
    the remaining bits. Beyond that, see section 6.3.4 of the [paper].

    [paper](http://www.pcg-random.org/paper.html)

    Once we have 32 random bits, we have to turn it into a number. For integers,
    we first check if the range is a power of two. If it is, we can mask part of
    the value and be done. If not, we need to account for bias.

    Let's say you want a random number between 1 and 7 but I can only generate
    random numbers between 1 and 32. If I modulus by result by 7, I'm biased,
    because there are more random numbers that lead to 1 than 7. So instead, I
    check to see if my random number exceeds 28 (the largest multiple of 7 less
    than 32). If it does, I reroll, otherwise I mod by seven. This sounds
    wateful, except that instead of 32 it's 2^32, so in practice it's hard to
    notice. So that's how we get random ints. There's another process from
    floats, but I don't understand it very well.
-}


{-| Maybe you do not want to use [`generate`](#generate) for some reason? Maybe
you need to be able to exactly reproduce a sequence of random values?

In that case, you can work with a `Seed` of randomness and [`step`](#step) it
forward by hand.
-}
type Seed
    = Seed Int Int
    -- the first Int is the state of the RNG and stepped with each random generation
    -- the second state is the increment which corresponds to an independent RNG


-- step the RNG to produce the next seed
-- this is incredibly simple: multiply the state by a constant factor, modulus it
-- by 2^32, and add a magic addend. The addend can be varied to produce independent
-- RNGs, so it is stored as part of the seed. It is given to the new seed unchanged.
next : Seed -> Seed
next (Seed state0 incr) =
    -- The magic constant is from Numerical Recipes and is inlined for perf.
    Seed (Bitwise.shiftRightZfBy 0 ((state0 * 1664525) + incr)) incr


-- obtain a psuedorandom 32-bit integer from a seed
peel : Seed -> Int
peel (Seed state _) =
    -- This is the RXS-M-SH version of PCG, see section 6.3.4 of the paper
    -- and line 184 of pcg_variants.h in the 0.94 (non-minimal) C implementation,
    -- the latter of which is the source of the magic constant.
    let
        word =
            (Bitwise.xor state (Bitwise.shiftRightZfBy ((Bitwise.shiftRightZfBy 28 state) + 4) state)) * 277803737
    in
        Bitwise.shiftRightZfBy 0 (Bitwise.xor (Bitwise.shiftRightZfBy 22 word) word)


{-| A `Generator` is a **recipe** for generating random values. For example,
here is a generator for numbers between 1 and 10 inclusive:

    import Random

    oneToTen : Random.Generator Int
    oneToTen =
      Random.int 1 10

Notice that we are not actually generating any numbers yet! We are describing
what kind of values we want. To actually get random values, you create a
command with the [`generate`](#generate) function:

    type Msg = NewNumber Int

    newNumber : Cmd Msg
    newNumber =
      Random.generate NewNumber oneToTen

Each time you run this command, it runs the `oneToTen` generator and produces
random integers between one and ten.

**Note 1:** If you are not familiar with commands yet, start working through
[guide.elm-lang.org][guide]. It builds up to an example that generates
random numbers. Commands are one of the core concepts in Elm, and it will
be faster overall to invest in understanding them _now_ rather than copy/pasting
your way to frustration! And if you feel stuck on something, definitely ask
about it in [the Elm slack][slack]. Folks are happy to help!

**Note 2:** The random `Generator` API is quite similar to the JSON `Decoder` API.
Both are building blocks that snap together with `map`, `map2`, etc. You can read
more about JSON decoders [here][json] to see the similarity.

[guide]: https://guide.elm-lang.org/
[slack]: https://elmlang.herokuapp.com/
[json]: https://guide.elm-lang.org/interop/json.html
-}
type Generator a =
    Generator (Seed -> (a, Seed))


{-| So you need _reproducable_ randomness for some reason.

This `step` function lets you use a `Generator` without commands. It is a
normal Elm function. Same input, same output! So to get a 3D point you could
say:

    import Random

    type alias Point3D = { x : Float, y : Float, z : Float }

    point3D : Random.Seed -> (Point3D, Random.Seed)
    point3D seed0 =
      let
        (x, seed1) = Random.step (Random.int 0 100) seed0
        (y, seed2) = Random.step (Random.int 0 100) seed1
        (z, seed3) = Random.step (Random.int 0 100) seed2
      in
        ( Point3D x y z, seed3 )

Notice that we use different seeds on each line! If we instead used `seed0`
for everything, the `x`, `y`, and `z` values would always be exactly the same!
Same input, same output!

Threading seeds around is not super fun, so if you really need this, it is
best to build your `Generator` like normal and then just `step` it all at
once at the top of your program.
-}
step : Generator a -> Seed -> (a, Seed)
step (Generator generator) seed =
  generator seed


{-| Create a `Seed` for _reproducable_ randomness.

    import Random

    seed0 : Random.Seed
    seed0 =
      Random.initialSeed 42

If you hard-code your `Seed` like this, every run will be the same. This can
be useful if you are testing a game with randomness and want it to be easy to
reproduce past games.

In practice, you may want to get the initial seed by (1) sending it to Elm
through flags or (2) using `Time.now` to get a number that the user has not
seen before. (Flags are described on [this page][flags].)

[flags]: https://guide.elm-lang.org/interop/javascript.html
-}
initialSeed : Int -> Seed
initialSeed x =
    let
        -- the default increment magic constant is taken from Numerical Recipes
        (Seed state1 incr) =
            next (Seed 0 1013904223)

        state2 =
            Bitwise.shiftRightZfBy 0 (state1 + x)
    in
        next (Seed state2 incr)


{-| A generator that produces a seed that is independent of any other seed in
the program. These seeds will generate their own unique sequences of random
values. They are useful when you need an unknown amount of randomness *later*
but can request only a fixed amount of randomness *now*.

The independent seeds are extremely likely to be distinct for all practical
purposes. However, it is not proven that there are no pathological cases.
-}
independentSeed : Generator Seed
independentSeed =
    Generator <|
        \seed0 ->
            let
                gen =
                    int 0 0xFFFFFFFF

                {--
                Although it probably doesn't hold water theoretically, xor two
                random numbers to make an increment less likely to be
                pathological. Then make sure that it's odd, which is required.
                Next make sure it is positive. Finally step it once before use.
                --}
                makeIndependentSeed state b c =
                    next <| Seed state <|
                        Bitwise.shiftRightZfBy 0 (Bitwise.or 1 (Bitwise.xor b c))
            in
            step (map3 makeIndependentSeed gen gen gen) seed0



-- MANAGER


{-| Create a command that produces random values. Say you want to generate
random points:

    import Random

    point : Random.Generator (Int, Int)
    point =
      Random.pair (Random.int -100 100) (Random.int -100 100)

    type Msg = NewPoint (Int, Int)

    newPoint : Cmd Msg
    newPoint =
      Random.generate NewPoint point

Each time you run the `newPoint` command, it will produce a new 2D point like
`(57, 18)` or `(-82, 6)`.

**Note:** Read through [guide.elm-lang.org][guide] to learn how commands work.
If you are coming from JS it can be hopelessly frustrating if you just try to
wing it. And definitely ask around on Slack if you feel stuck! Investing in
understanding generators is really worth it, and once it clicks, folks often
dread going back to `Math.random()` in JavaScript.

[guide]: https://guide.elm-lang.org/
-}
generate : (a -> msg) -> Generator a -> Cmd msg
generate tagger generator =
  command (Generate (map tagger generator))


type MyCmd msg = Generate (Generator msg)


cmdMap : (a -> b) -> MyCmd a -> MyCmd b
cmdMap func (Generate generator) =
  Generate (map func generator)


init : Task Never Seed
init =
  Task.andThen (\time -> Task.succeed (initialSeed (Time.posixToMillis time))) Time.now


onEffects : Platform.Router msg Never -> List (MyCmd msg) -> Seed -> Task Never Seed
onEffects router commands seed =
  case commands of
    [] ->
      Task.succeed seed

    Generate generator :: rest ->
      let
        (value, newSeed) =
          step generator seed
      in
          Task.andThen
            (\_ -> onEffects router rest newSeed)
            (Platform.sendToApp router value)


onSelfMsg : Platform.Router msg Never -> Never -> Seed -> Task Never Seed
onSelfMsg _ _ seed =
  Task.succeed seed
