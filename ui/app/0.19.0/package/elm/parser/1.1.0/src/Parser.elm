module Parser exposing
  ( Parser, run
  , int, float, number, symbol, keyword, variable, end
  , succeed, (|=), (|.), lazy, andThen, problem
  , oneOf, map, backtrackable, commit, token
  , sequence, Trailing(..), loop, Step(..)
  , spaces, lineComment, multiComment, Nestable(..)
  , getChompedString, chompIf, chompWhile, chompUntil, chompUntilEndOr, mapChompedString
  , DeadEnd, Problem(..), deadEndsToString
  , withIndent, getIndent
  , getPosition, getRow, getCol, getOffset, getSource
  )


{-|

# Parsers
@docs Parser, run

# Building Blocks
@docs int, float, number, symbol, keyword, variable, end

# Pipelines
@docs succeed, (|=), (|.), lazy, andThen, problem

# Branches
@docs oneOf, map, backtrackable, commit, token

# Loops
@docs sequence, Trailing, loop, Step

# Whitespace
@docs spaces, lineComment, multiComment, Nestable

# Chompers
@docs getChompedString, chompIf, chompWhile, chompUntil, chompUntilEndOr, mapChompedString

# Errors
@docs DeadEnd, Problem, deadEndsToString

# Indentation
@docs withIndent, getIndent

# Positions
@docs getPosition, getRow, getCol, getOffset, getSource
-}


import Char
import Parser.Advanced as A exposing ((|=), (|.))
import Set



-- INFIX OPERATORS - see Parser.Advanced for why 5 and 6 were chosen


infix left 5 (|=) = keeper
infix left 6 (|.) = ignorer



-- PARSERS


{-| A `Parser` helps turn a `String` into nicely structured data. For example,
we can [`run`](#run) the [`int`](#int) parser to turn `String` to `Int`:

    run int "123456" == Ok 123456
    run int "3.1415" == Err ...

The cool thing is that you can combine `Parser` values to handle much more
complex scenarios.
-}
type alias Parser a =
  A.Parser Never Problem a



-- RUN


{-| Try a parser. Here are some examples using the [`keyword`](#keyword)
parser:

    run (keyword "true") "true"  == Ok ()
    run (keyword "true") "True"  == Err ...
    run (keyword "true") "false" == Err ...
    run (keyword "true") "true!" == Ok ()

Notice the last case! A `Parser` will chomp as much as possible and not worry
about the rest. Use the [`end`](#end) parser to ensure you made it to the end
of the string!
-}
run : Parser a -> String -> Result (List DeadEnd) a
run parser source =
  case A.run parser source of
    Ok a ->
      Ok a

    Err problems ->
      Err (List.map problemToDeadEnd problems)


problemToDeadEnd : A.DeadEnd Never Problem -> DeadEnd
problemToDeadEnd p =
  DeadEnd p.row p.col p.problem



-- PROBLEMS


{-| A parser can run into situations where there is no way to make progress.
When that happens, I record the `row` and `col` where you got stuck and the
particular `problem` you ran into. That is a `DeadEnd`!

**Note:** I count rows and columns like a text editor. The beginning is `row=1`
and `col=1`. As I chomp characters, the `col` increments. When I reach a `\n`
character, I increment the `row` and set `col=1`.
-}
type alias DeadEnd =
  { row : Int
  , col : Int
  , problem : Problem
  }


{-| When you run into a `DeadEnd`, I record some information about why you
got stuck. This data is useful for producing helpful error messages. This is
how [`deadEndsToString`](#deadEndsToString) works!

**Note:** If you feel limited by this type (i.e. having to represent custom
problems as strings) I highly recommend switching to `Parser.Advanced`. It
lets you define your own `Problem` type. It can also track "context" which
can improve error messages a ton! This is how the Elm compiler produces
relatively nice parse errors, and I am excited to see those techniques applied
elsewhere!
-}
type Problem
  = Expecting String
  | ExpectingInt
  | ExpectingHex
  | ExpectingOctal
  | ExpectingBinary
  | ExpectingFloat
  | ExpectingNumber
  | ExpectingVariable
  | ExpectingSymbol String
  | ExpectingKeyword String
  | ExpectingEnd
  | UnexpectedChar
  | Problem String
  | BadRepeat


{-| Turn all the `DeadEnd` data into a string that is easier for people to
read.

**Note:** This is just a baseline of quality. It cannot do anything with colors.
It is not interactivite. It just turns the raw data into strings. I really hope
folks will check out the source code for some inspiration on how to turn errors
into `Html` with nice colors and interaction! The `Parser.Advanced` module lets
you work with context as well, which really unlocks another level of quality!
The "context" technique is how the Elm compiler can say "I think I am parsing a
list, so I was expecting a closing `]` here." Telling users what the parser
_thinks_ is happening can be really helpful!
-}
deadEndsToString : List DeadEnd -> String
deadEndsToString deadEnds =
  "TODO deadEndsToString"



-- PIPELINES


{-| A parser that succeeds without chomping any characters.

    run (succeed 90210  ) "mississippi" == Ok 90210
    run (succeed 3.141  ) "mississippi" == Ok 3.141
    run (succeed ()     ) "mississippi" == Ok ()
    run (succeed Nothing) "mississippi" == Ok Nothing

Seems weird on its own, but it is very useful in combination with other
functions. The docs for [`(|=)`](#|=) and [`andThen`](#andThen) have some neat
examples.
-}
succeed : a -> Parser a
succeed =
  A.succeed


{-| **Keep** values in a parser pipeline. For example, we could say:

    type alias Point = { x : Float, y : Float }

    point : Parser Point
    point =
      succeed Point
        |. symbol "("
        |. spaces
        |= float
        |. spaces
        |. symbol ","
        |. spaces
        |= float
        |. spaces
        |. symbol ")"

All the parsers in this pipeline will chomp characters and produce values. So
`symbol "("` will chomp one paren and produce a `()` value. Similarly, `float`
will chomp some digits and produce a `Float` value. The `(|.)` and `(|=)`
operators just decide whether we give the values to the `Point` function.

So in this case, we skip the `()` from `symbol "("`, we skip the `()` from
`spaces`, we keep the `Float` from `float`, etc.
-}
keeper : Parser (a -> b) -> Parser a -> Parser b
keeper =
  (|=)


{-| **Skip** values in a parser pipeline. For example, maybe we want to parse
some JavaScript variables:

    var : Parser String
    var =
      getChompedString <|
        succeed ()
          |. chompIf isStartChar
          |. chompWhile isInnerChar

    isStartChar : Char -> Bool
    isStartChar char =
      Char.isAlpha char || char == '_' || char == '$'

    isInnerChar : Char -> Bool
    isInnerChar char =
      isStartChar char || Char.isDigit char

`chompIf isStartChar` can chomp one character and produce a `()` value.
`chompWhile isInnerChar` can chomp zero or more characters and produce a `()`
value. The `(|.)` operators are saying to still chomp all the characters, but
skip the two `()` values that get produced. No one cares about them.
-}
ignorer : Parser keep -> Parser ignore -> Parser keep
ignorer =
  (|.)


{-| Helper to define recursive parsers. Say we want a parser for simple
boolean expressions:

    true
    false
    (true || false)
    (true || (true || false))

Notice that a boolean expression might contain *other* boolean expressions.
That means we will want to define our parser in terms of itself:

    type Boolean
      = MyTrue
      | MyFalse
      | MyOr Boolean Boolean

    boolean : Parser Boolean
    boolean =
      oneOf
        [ succeed MyTrue
            |. keyword "true"
        , succeed MyFalse
            |. keyword "false"
        , succeed MyOr
            |. symbol "("
            |. spaces
            |= lazy (\_ -> boolean)
            |. spaces
            |. symbol "||"
            |. spaces
            |= lazy (\_ -> boolean)
            |. spaces
            |. symbol ")"
        ]

**Notice that `boolean` uses `boolean` in its definition!** In Elm, you can
only define a value in terms of itself it is behind a function call. So
`lazy` helps us define these self-referential parsers. (`andThen` can be used
for this as well!)
-}
lazy : (() -> Parser a) -> Parser a
lazy =
  A.lazy


{-| Parse one thing `andThen` parse another thing. This is useful when you want
to check on what you just parsed. For example, maybe you want U.S. zip codes
and `int` is not suitable because it does not allow leading zeros. You could
say:

    zipCode : Parser String
    zipCode =
      getChompedString (chompWhile Char.isDigit)
        |> andThen checkZipCode

    checkZipCode : String -> Parser String
    checkZipCode code =
      if String.length code == 5 then
        succeed code
      else
        problem "a U.S. zip code has exactly 5 digits"

First we chomp digits `andThen` we check if it is a valid U.S. zip code. We
`succeed` if it has exactly five digits and report a `problem` if not.

Check out [`examples/DoubleQuoteString.elm`](https://github.com/elm/parser/blob/master/examples/DoubleQuoteString.elm)
for another example, this time using `andThen` to verify unicode code points.

**Note:** If you are using `andThen` recursively and blowing the stack, check
out the [`loop`](#loop) function to limit stack usage.
-}
andThen : (a -> Parser b) -> Parser a -> Parser b
andThen =
  A.andThen


{-| Indicate that a parser has reached a dead end. "Everything was going fine
until I ran into this problem." Check out the [`andThen`](#andThen) docs to see
an example usage.
-}
problem : String -> Parser a
problem msg =
  A.problem (Problem msg)



-- BACKTRACKING


{-| If you are parsing JSON, the values can be strings, floats, booleans,
arrays, objects, or null. You need a way to pick `oneOf` them! Here is a
sample of what that code might look like:

    type Json
      = Number Float
      | Boolean Bool
      | Null

    json : Parser Json
    json =
      oneOf
        [ map Number float
        , map (\_ -> Boolean True) (keyword "true")
        , map (\_ -> Boolean False) (keyword "false")
        , map (\_ -> Null) keyword "null"
        ]

This parser will keep trying parsers until `oneOf` them starts chomping
characters. Once a path is chosen, it does not come back and try the others.

**Note:** I highly recommend reading [this document][semantics] to learn how
`oneOf` and `backtrackable` interact. It is subtle and important!

[semantics]: https://github.com/elm/parser/blob/master/semantics.md
-}
oneOf : List (Parser a) -> Parser a
oneOf =
  A.oneOf


{-| Transform the result of a parser. Maybe you have a value that is
an integer or `null`:

    nullOrInt : Parser (Maybe Int)
    nullOrInt =
      oneOf
        [ map Just int
        , map (\_ -> Nothing) (keyword "null")
        ]

    -- run nullOrInt "0"    == Ok (Just 0)
    -- run nullOrInt "13"   == Ok (Just 13)
    -- run nullOrInt "null" == Ok Nothing
    -- run nullOrInt "zero" == Err ...
-}
map : (a -> b) -> Parser a -> Parser b
map =
  A.map


{-| It is quite tricky to use `backtrackable` well! It can be very useful, but
also can degrade performance and error message quality.

Read [this document](https://github.com/elm/parser/blob/master/semantics.md)
to learn how `oneOf`, `backtrackable`, and `commit` work and interact with
each other. It is subtle and important!
-}
backtrackable : Parser a -> Parser a
backtrackable =
  A.backtrackable


{-| `commit` is almost always paired with `backtrackable` in some way, and it
is tricky to use well.

Read [this document](https://github.com/elm/parser/blob/master/semantics.md)
to learn how `oneOf`, `backtrackable`, and `commit` work and interact with
each other. It is subtle and important!
-}
commit : a -> Parser a
commit =
  A.commit



-- TOKEN


{-| Parse exactly the given string, without any regard to what comes next.

A potential pitfall when parsing keywords is getting tricked by variables that
start with a keyword, like `let` in `letters` or `import` in `important`. This
is especially likely if you have a whitespace parser that can consume zero
charcters. So the [`keyword`](#keyword) parser is defined with `token` and a
trick to peek ahead a bit:

    keyword : String -> Parser ()
    keyword kwd =
      succeed identity
        |. backtrackable (token kwd)
        |= oneOf
            [ map (\_ -> True) (backtrackable (chompIf isVarChar))
            , succeed False
            ]
        |> andThen (checkEnding kwd)

    checkEnding : String -> Bool -> Parser ()
    checkEnding kwd isBadEnding =
      if isBadEnding then
        problem ("expecting the `" ++ kwd ++ "` keyword")
      else
        commit ()

    isVarChar : Char -> Bool
    isVarChar char =
      Char.isAlphaNum char || char == '_'

This definition is specially designed so that (1) if you really see `let` you
commit to that path and (2) if you see `letters` instead you can backtrack and
try other options. If I had just put a `backtrackable` around the whole thing
you would not get (1) anymore.
-}
token : String -> Parser ()
token str =
  A.token (toToken str)


toToken : String -> A.Token Problem
toToken str =
  A.Token str (Expecting str)



-- LOOPS


{-| A parser that can loop indefinitely. This can be helpful when parsing
repeated structures, like a bunch of statements:

    statements : Parser (List Stmt)
    statements =
      loop [] statementsHelp

    statementsHelp : List Stmt -> Parser (Step (List Stmt) (List Stmt))
    statementsHelp revStmts =
      oneOf
        [ succeed (\stmt -> Loop (stmt :: revStmts))
            |= statement
            |. spaces
            |. symbol ";"
            |. spaces
        , succeed ()
            |> map (\_ -> Done (List.reverse revStmts))
        ]

    -- statement : Parser Stmt

Notice that the statements are tracked in reverse as we `Loop`, and we reorder
them only once we are `Done`. This is a very common pattern with `loop`!

Check out [`examples/DoubleQuoteString.elm`](https://github.com/elm/parser/blob/master/examples/DoubleQuoteString.elm)
for another example.

**IMPORTANT NOTE:** Parsers like `succeed ()` and `chompWhile Char.isAlpha` can
succeed without consuming any characters. So in some cases you may want to use
[`getOffset`](#getOffset) to ensure that each step actually consumed characters.
Otherwise you could end up in an infinite loop!

**Note:** Anything you can write with `loop`, you can also write as a parser
that chomps some characters `andThen` calls itself with new arguments. The
problem with calling `andThen` recursively is that it grows the stack, so you
cannot do it indefinitely. So `loop` is important because enables tail-call
elimination, allowing you to parse however many repeats you want.
-}
loop : state -> (state -> Parser (Step state a)) -> Parser a
loop state callback =
  A.loop state (\s -> map toAdvancedStep (callback s))


{-| Decide what steps to take next in your [`loop`](#loop).

If you are `Done`, you give the result of the whole `loop`. If you decide to
`Loop` around again, you give a new state to work from. Maybe you need to add
an item to a list? Or maybe you need to track some information about what you
just saw?

**Note:** It may be helpful to learn about [finite-state machines][fsm] to get
a broader intuition about using `state`. I.e. You may want to create a `type`
that describes four possible states, and then use `Loop` to transition between
them as you consume characters.

[fsm]: https://en.wikipedia.org/wiki/Finite-state_machine
-}
type Step state a
  = Loop state
  | Done a


toAdvancedStep : Step s a -> A.Step s a
toAdvancedStep step =
  case step of
    Loop s -> A.Loop s
    Done a -> A.Done a



-- NUMBERS


{-| Parse integers.

    run int "1"    == Ok 1
    run int "1234" == Ok 1234

    run int "-789" == Err ...
    run int "0123" == Err ...
    run int "1.34" == Err ...
    run int "1e31" == Err ...
    run int "123a" == Err ...
    run int "0x1A" == Err ...

If you want to handle a leading `+` or `-` you should do it with a custom
parser like this:

    myInt : Parser Int
    myInt =
      oneOf
        [ succeed negate
            |. symbol "-"
            |= int
        , int
        ]

**Note:** If you want a parser for both `Int` and `Float` literals, check out
[`number`](#number) below. It will be faster than using `oneOf` to combining
`int` and `float` yourself.
-}
int : Parser Int
int =
  A.int ExpectingInt ExpectingInt


{-| Parse floats.

    run float "123"       == Ok 123
    run float "3.1415"    == Ok 3.1415
    run float "0.1234"    == Ok 0.1234
    run float ".1234"     == Ok 0.1234
    run float "1e-42"     == Ok 1e-42
    run float "6.022e23"  == Ok 6.022e23
    run float "6.022E23"  == Ok 6.022e23
    run float "6.022e+23" == Ok 6.022e23

If you want to disable literals like `.123` (like in Elm) you could write
something like this:

    elmFloat : Parser Float
    elmFloat =
      oneOf
        [ symbol "."
            |. problem "floating point numbers must start with a digit, like 0.25"
        , float
        ]

**Note:** If you want a parser for both `Int` and `Float` literals, check out
[`number`](#number) below. It will be faster than using `oneOf` to combining
`int` and `float` yourself.
-}
float : Parser Float
float =
  A.float ExpectingFloat ExpectingFloat



-- NUMBER


{-| Parse a bunch of different kinds of numbers without backtracking. A parser
for Elm would need to handle integers, floats, and hexadecimal like this:

    type Expr
      = Variable String
      | Int Int
      | Float Float
      | Apply Expr Expr

    elmNumber : Parser Expr
    elmNumber =
      number
        { int = Just Int
        , hex = Just Int    -- 0x001A is allowed
        , octal = Nothing   -- 0o0731 is not
        , binary = Nothing  -- 0b1101 is not
        , float = Just Float
        }

If you wanted to implement the [`float`](#float) parser, it would be like this:

    float : Parser Float
    float =
      number
        { int = Just toFloat
        , hex = Nothing
        , octal = Nothing
        , binary = Nothing
        , float = Just identity
        }

Notice that it actually is processing `int` results! This is because `123`
looks like an integer to me, but maybe it looks like a float to you. If you had
`int = Nothing`, floats would need a decimal like `1.0` in every case. If you
like explicitness, that may actually be preferable!

**Note:** This function does not check for weird trailing characters in the
current implementation, so parsing `123abc` can succeed up to `123` and then
move on. This is helpful for people who want to parse things like `40px` or
`3m`, but it requires a bit of extra code to rule out trailing characters in
other cases.
-}
number
  : { int : Maybe (Int -> a)
    , hex : Maybe (Int -> a)
    , octal : Maybe (Int -> a)
    , binary : Maybe (Int -> a)
    , float : Maybe (Float -> a)
    }
  -> Parser a
number i =
  A.number
    { int = Result.fromMaybe ExpectingInt i.int
    , hex = Result.fromMaybe ExpectingHex i.hex
    , octal = Result.fromMaybe ExpectingOctal i.octal
    , binary = Result.fromMaybe ExpectingBinary i.binary
    , float = Result.fromMaybe ExpectingFloat i.float
    , invalid = ExpectingNumber
    , expecting = ExpectingNumber
    }



-- SYMBOL


{-| Parse symbols like `(` and `,`.

    run (symbol "[") "[" == Ok ()
    run (symbol "[") "4" == Err ... (ExpectingSymbol "[") ...

**Note:** This is good for stuff like brackets and semicolons, but it probably
should not be used for binary operators like `+` and `-` because you can find
yourself in weird situations. For example, is `3--4` a typo? Or is it `3 - -4`?
I have had better luck with `chompWhile isSymbol` and sorting out which
operator it is afterwards.
-}
symbol : String -> Parser ()
symbol str =
  A.symbol (A.Token str (ExpectingSymbol str))



-- KEYWORD


{-| Parse keywords like `let`, `case`, and `type`.

    run (keyword "let") "let"     == Ok ()
    run (keyword "let") "var"     == Err ... (ExpectingKeyword "let") ...
    run (keyword "let") "letters" == Err ... (ExpectingKeyword "let") ...

**Note:** Notice the third case there! `keyword` actually looks ahead one
character to make sure it is not a letter, number, or underscore. The goal is
to help with parsers like this:

    succeed identity
      |. keyword "let"
      |. spaces
      |= elmVar
      |. spaces
      |. symbol "="

The trouble is that `spaces` may chomp zero characters (to handle expressions
like `[1,2]` and `[ 1 , 2 ]`) and in this case, it would mean `letters` could
be parsed as `let ters` and then wonder where the equals sign is! Check out the
[`token`](#token) docs if you need to customize this!
-}
keyword : String -> Parser ()
keyword kwd =
  A.keyword (A.Token kwd (ExpectingKeyword kwd))



-- END


{-| Check if you have reached the end of the string you are parsing.

    justAnInt : Parser Int
    justAnInt =
      succeed identity
        |= int
        |. end

    -- run justAnInt "90210" == Ok 90210
    -- run justAnInt "1 + 2" == Err ...
    -- run int       "1 + 2" == Ok 1

Parsers can succeed without parsing the whole string. Ending your parser
with `end` guarantees that you have successfully parsed the whole string.
-}
end : Parser ()
end =
  A.end ExpectingEnd



-- CHOMPED STRINGS


{-| Sometimes parsers like `int` or `variable` cannot do exactly what you
need. The "chomping" family of functions is meant for that case! Maybe you
need to parse [valid PHP variables][php] like `$x` and `$txt`:

    php : Parser String
    php =
      getChompedString <|
        succeed ()
          |. chompIf (\c -> c == '$')
          |. chompIf (\c -> Char.isAlpha c || c == '_')
          |. chompWhile (\c -> Char.isAlphaNum c || c == '_')

The idea is that you create a bunch of chompers that validate the underlying
characters. Then `getChompedString` extracts the underlying `String` efficiently.

**Note:** Maybe it is helpful to see how you can use [`getOffset`](#getOffset)
and [`getSource`](#getSource) to implement this function:

    getChompedString : Parser a -> Parser String
    getChompedString parser =
      succeed String.slice
        |= getOffset
        |. parser
        |= getOffset
        |= getSource

[php]: https://www.w3schools.com/php/php_variables.asp
-}
getChompedString : Parser a -> Parser String
getChompedString =
  A.getChompedString


{-| This works just like [`getChompedString`](#getChompedString) but gives
a bit more flexibility. For example, maybe you want to parse Elm doc comments
and get (1) the full comment and (2) all of the names listed in the docs.

You could implement `mapChompedString` like this:

    mapChompedString : (String -> a -> b) -> Parser a -> Parser String
    mapChompedString func parser =
      succeed (\start value end src -> func (String.slice start end src) value)
        |= getOffset
        |= parser
        |= getOffset
        |= getSource

-}
mapChompedString : (String -> a -> b) -> Parser a -> Parser b
mapChompedString =
  A.mapChompedString



{-| Chomp one character if it passes the test.

    chompUpper : Parser ()
    chompUpper =
      chompIf Char.isUpper

So this can chomp a character like `T` and produces a `()` value.
-}
chompIf : (Char -> Bool) -> Parser ()
chompIf isGood =
  A.chompIf isGood UnexpectedChar



{-| Chomp zero or more characters if they pass the test. This is commonly
useful for chomping whitespace or variable names:

    whitespace : Parser ()
    whitespace =
      chompWhile (\c -> c == ' ' || c == '\t' || c == '\n' || c == '\r')

    elmVar : Parser String
    elmVar =
      getChompedString <|
        succeed ()
          |. chompIf Char.isLower
          |. chompWhile (\c -> Char.isAlphaNum c || c == '_')

**Note:** a `chompWhile` parser always succeeds! This can lead to tricky
situations, especially if you define your whitespace with it. In that case,
you could accidentally interpret `letx` as the keyword `let` followed by
"spaces" followed by the variable `x`. This is why the `keyword` and `number`
parsers peek ahead, making sure they are not followed by anything unexpected.
-}
chompWhile : (Char -> Bool) -> Parser ()
chompWhile =
  A.chompWhile


{-| Chomp until you see a certain string. You could define C-style multi-line
comments like this:

    comment : Parser ()
    comment =
      symbol "/*"
        |. chompUntil "*/"

I recommend using [`multiComment`](#multiComment) for this particular scenario
though. It can be trickier than it looks!
-}
chompUntil : String -> Parser ()
chompUntil str =
  A.chompUntil (toToken str)


{-| Chomp until you see a certain string or until you run out of characters to
chomp! You could define single-line comments like this:

    elm : Parser ()
    elm =
      symbol "--"
        |. chompUntilEndOr "\n"

A file may end with a single-line comment, so the file can end before you see
a newline. Tricky!

I recommend just using [`lineComment`](#lineComment) for this particular
scenario.
-}
chompUntilEndOr : String -> Parser ()
chompUntilEndOr =
  A.chompUntilEndOr



-- INDENTATION


{-| Some languages are indentation sensitive. Python cares about tabs. Elm
cares about spaces sometimes. `withIndent` and `getIndent` allow you to manage
"indentation state" yourself, however is necessary in your scenario.
-}
withIndent : Int -> Parser a -> Parser a
withIndent =
  A.withIndent


{-| When someone said `withIndent` earlier, what number did they put in there?

- `getIndent` results in `0`, the default value
- `withIndent 4 getIndent` results in `4`

So you are just asking about things you said earlier. These numbers do not leak
out of `withIndent`, so say we have:

    succeed Tuple.pair
      |= withIndent 4 getIndent
      |= getIndent

Assuming there are no `withIndent` above this, you would get `(4,0)` from this.
-}
getIndent : Parser Int
getIndent =
  A.getIndent



-- POSITION


{-| Code editors treat code like a grid, with rows and columns. The start is
`row=1` and `col=1`. As you chomp characters, the `col` increments. When you
run into a `\n` character, the `row` increments and `col` goes back to `1`.

In the Elm compiler, I track the start and end position of every expression
like this:

    type alias Located a =
      { start : (Int, Int)
      , value : a
      , end : (Int, Int)
      }

    located : Parser a -> Parser (Located a)
    located parser =
      succeed Located
        |= getPosition
        |= parser
        |= getPosition

So if there is a problem during type inference, I use this saved position
information to underline the exact problem!

**Note:** Tabs count as one character, so if you are parsing something like
Python, I recommend sorting that out *after* parsing. So if I wanted the `^^^^`
underline like in Elm, I would find the `row` in the source code and do
something like this:

    makeUnderline : String -> Int -> Int -> String
    makeUnderline row minCol maxCol =
      String.toList row
        |> List.indexedMap (toUnderlineChar minCol maxCol)
        |> String.fromList

    toUnderlineChar : Int -> Int -> Int -> Char -> Char
    toUnderlineChar minCol maxCol col char =
      if minCol <= col && col <= maxCol then
        '^'
      else if char == '\t' then
        '\t'
      else
        ' '

So it would preserve any tabs from the source line. There are tons of other
ways to do this though. The point is just that you handle the tabs after
parsing but before anyone looks at the numbers in a context where tabs may
equal 2, 4, or 8.
-}
getPosition : Parser (Int, Int)
getPosition =
  A.getPosition


{-| This is a more efficient version of `map Tuple.first getPosition`. Maybe
you just want to track the line number for some reason? This lets you do that.

See [`getPosition`](#getPosition) for an explanation of rows and columns.
-}
getRow : Parser Int
getRow =
  A.getRow


{-| This is a more efficient version of `map Tuple.second getPosition`. This
can be useful in combination with [`withIndent`](#withIndent) and
[`getIndent`](#getIndent), like this:

    checkIndent : Parser ()
    checkIndent =
      succeed (\indent column -> indent <= column)
        |= getIndent
        |= getCol
        |> andThen checkIndentHelp

    checkIndentHelp : Bool -> Parser ()
    checkIndentHelp isIndented =
      if isIndented then
        succeed ()
      else
        problem "expecting more spaces"

So the `checkIndent` parser only succeeds when you are "deeper" than the
current indent level. You could use this to parse Elm-style `let` expressions.
-}
getCol : Parser Int
getCol =
  A.getCol


{-| Editors think of code as a grid, but behind the scenes it is just a flat
array of UTF-16 characters. `getOffset` tells you your index in that flat
array. So if you chomp `"\n\n\n\n"` you are on row 5, column 1, and offset 4.

**Note:** JavaScript uses a somewhat odd version of UTF-16 strings, so a single
character may take two slots. So in JavaScript, `'abc'.length === 3` but
`'🙈🙉🙊'.length === 6`. Try it out! And since Elm runs in JavaScript, the offset
moves by those rules.
-}
getOffset : Parser Int
getOffset =
  A.getOffset


{-| Get the full string that is being parsed. You could use this to define
`getChompedString` or `mapChompedString` if you wanted:

    getChompedString : Parser a -> Parser String
    getChompedString parser =
      succeed String.slice
        |= getOffset
        |. parser
        |= getOffset
        |= getSource
-}
getSource : Parser String
getSource =
  A.getSource



-- VARIABLES


{-| Create a parser for variables. If we wanted to parse type variables in Elm,
we could try something like this:

    import Char
    import Parser exposing (..)
    import Set

    typeVar : Parser String
    typeVar =
      variable
        { start = Char.isLower
        , inner = \c -> Char.isAlphaNum c || c == '_'
        , reserved = Set.fromList [ "let", "in", "case", "of" ]
        }

This is saying it _must_ start with a lower-case character. After that,
characters can be letters, numbers, or underscores. It is also saying that if
you run into any of these reserved names, it is definitely not a variable.
-}
variable :
  { start : Char -> Bool
  , inner : Char -> Bool
  , reserved : Set.Set String
  }
  -> Parser String
variable i =
  A.variable
    { start = i.start
    , inner = i.inner
    , reserved = i.reserved
    , expecting = ExpectingVariable
    }



-- SEQUENCES


{-| Handle things like lists and records, but you can customize the details
however you need. Say you want to parse C-style code blocks:

    import Parser exposing (Parser, Trailing(..))

    block : Parser (List Stmt)
    block =
      Parser.sequence
        { start = "{"
        , separator = ";"
        , end = "}"
        , spaces = spaces
        , item = statement
        , trailing = Mandatory -- demand a trailing semi-colon
        }

    -- statement : Parser Stmt

**Note:** If you need something more custom, do not be afraid to check
out the implementation and customize it for your case. It is better to
get nice error messages with a lower-level implementation than to try
to hack high-level parsers to do things they are not made for.
-}
sequence
  : { start : String
    , separator : String
    , end : String
    , spaces : Parser ()
    , item : Parser a
    , trailing : Trailing
    }
  -> Parser (List a)
sequence i =
  A.sequence
    { start = toToken i.start
    , separator = toToken i.separator
    , end = toToken i.end
    , spaces = i.spaces
    , item = i.item
    , trailing = toAdvancedTrailing i.trailing
    }


{-| What’s the deal with trailing commas? Are they `Forbidden`?
Are they `Optional`? Are they `Mandatory`? Welcome to [shapes
club](https://poorlydrawnlines.com/comic/shapes-club/)!
-}
type Trailing = Forbidden | Optional | Mandatory


toAdvancedTrailing : Trailing -> A.Trailing
toAdvancedTrailing trailing =
  case trailing of
    Forbidden -> A.Forbidden
    Optional -> A.Optional
    Mandatory -> A.Mandatory



-- WHITESPACE


{-| Parse zero or more `' '`, `'\n'`, and `'\r'` characters.

The implementation is pretty simple:

    spaces : Parser ()
    spaces =
      chompWhile (\c -> c == ' ' || c == '\n' || c == '\r')

So if you need something different (like tabs) just define an alternative with
the necessary tweaks! Check out [`lineComment`](#lineComment) and
[`multiComment`](#multiComment) for more complex situations.
-}
spaces : Parser ()
spaces =
  A.spaces


{-| Parse single-line comments:

    elm : Parser ()
    elm =
      lineComment "--"

    js : Parser ()
    js =
      lineComment "//"

    python : Parser ()
    python =
      lineComment "#"

This parser is defined like this:

    lineComment : String -> Parser ()
    lineComment str =
      symbol str
        |. chompUntilEndOr "\n"

So it will consume the remainder of the line. If the file ends before you see
a newline, that is fine too.
-}
lineComment : String -> Parser ()
lineComment str =
  A.lineComment (toToken str)


{-| Parse multi-line comments. So if you wanted to parse Elm whitespace or
JS whitespace, you could say:

    elm : Parser ()
    elm =
      loop 0 <| ifProgress <|
        oneOf
          [ lineComment "--"
          , multiComment "{-" "-}" Nestable
          , spaces
          ]

    js : Parser ()
    js =
      loop 0 <| ifProgress <|
        oneOf
          [ lineComment "//"
          , multiComment "/*" "*/" NotNestable
          , chompWhile (\c -> c == ' ' || c == '\n' || c == '\r' || c == '\t')
          ]

    ifProgress : Parser a -> Int -> Parser (Step Int ())
    ifProgress parser offset =
      succeed identity
        |. parser
        |= getOffset
        |> map (\newOffset -> if offset == newOffset then Done () else Loop newOffset)

**Note:** The fact that `spaces` comes last in the definition of `elm` is very
important! It can succeed without consuming any characters, so if it were the
first option, it would always succeed and bypass the others! (Same is true of
`chompWhile` in `js`.) This possibility of success without consumption is also
why wee need the `ifProgress` helper. It detects if there is no more whitespace
to consume.
-}
multiComment : String -> String -> Nestable -> Parser ()
multiComment open close nestable =
  A.multiComment (toToken open) (toToken close) (toAdvancedNestable nestable)


{-| Not all languages handle multi-line comments the same. Multi-line comments
in C-style syntax are `NotNestable`, meaning they can be implemented like this:

    js : Parser ()
    js =
      symbol "/*"
        |. chompUntil "*/"

In fact, `multiComment "/*" "*/" NotNestable` *is* implemented like that! It is
very simple, but it does not allow you to nest comments like this:

```javascript
/*
line1
/* line2 */
line3
*/
```

It would stop on the first `*/`, eventually throwing a syntax error on the
second `*/`. This can be pretty annoying in long files.

Languages like Elm allow you to nest multi-line comments, but your parser needs
to be a bit fancier to handle this. After you start a comment, you have to
detect if there is another one inside it! And then you have to make sure all
the `{-` and `-}` match up properly! Saying `multiComment "{-" "-}" Nestable`
does all that for you.
-}
type Nestable = NotNestable | Nestable


toAdvancedNestable : Nestable -> A.Nestable
toAdvancedNestable nestable =
  case nestable of
    NotNestable -> A.NotNestable
    Nestable -> A.Nestable
