module Json.Encode exposing
  ( Value
  , encode
  , string, int, float, bool, null
  , list, array, set
  , object, dict
  )

{-| Library for turning Elm values into Json values.

# Encoding
@docs encode, Value

# Primitives
@docs string, int, float, bool, null

# Arrays
@docs list, array, set

# Objects
@docs object, dict
-}


import Array exposing (Array)
import Dict exposing (Dict)
import Set exposing (Set)
import Elm.Kernel.Json



-- ENCODE


{-| Represents a JavaScript value.
-}
type Value = Value


{-| Convert a `Value` into a prettified string. The first argument specifies
the amount of indentation in the resulting string.

    import Json.Encode as Encode

    tom : Encode.Value
    tom =
        Encode.object
            [ ( "name", Encode.string "Tom" )
            , ( "age", Encode.int 42 )
            ]

    compact = Encode.encode 0 tom
    -- {"name":"Tom","age":42}

    readable = Encode.encode 4 tom
    -- {
    --     "name": "Tom",
    --     "age": 42
    -- }
-}
encode : Int -> Value -> String
encode =
    Elm.Kernel.Json.encode



-- PRIMITIVES


{-| Turn a `String` into a JSON string.

    import Json.Encode exposing (encode, string)

    -- encode 0 (string "")      == "\"\""
    -- encode 0 (string "abc")   == "\"abc\""
    -- encode 0 (string "hello") == "\"hello\""
-}
string : String -> Value
string =
    Elm.Kernel.Json.wrap


{-| Turn an `Int` into a JSON number.

    import Json.Encode exposing (encode, int)

    -- encode 0 (int 42) == "42"
    -- encode 0 (int -7) == "-7"
    -- encode 0 (int 0)  == "0"
-}
int : Int -> Value
int =
    Elm.Kernel.Json.wrap


{-| Turn a `Float` into a JSON number.

    import Json.Encode exposing (encode, float)

    -- encode 0 (float 3.14)     == "3.14"
    -- encode 0 (float 1.618)    == "1.618"
    -- encode 0 (float -42)      == "-42"
    -- encode 0 (float NaN)      == "null"
    -- encode 0 (float Infinity) == "null"

**Note:** Floating point numbers are defined in the [IEEE 754 standard][ieee]
which is hardcoded into almost all CPUs. This standard allows `Infinity` and
`NaN`. [The JSON spec][json] does not include these values, so we encode them
both as `null`.

[ieee]: https://en.wikipedia.org/wiki/IEEE_754
[json]: https://www.json.org/
-}
float : Float -> Value
float =
    Elm.Kernel.Json.wrap


{-| Turn a `Bool` into a JSON boolean.

    import Json.Encode exposing (encode, bool)

    -- encode 0 (bool True)  == "true"
    -- encode 0 (bool False) == "false"
-}
bool : Bool -> Value
bool =
    Elm.Kernel.Json.wrap



-- NULLS


{-| Create a JSON `null` value.

    import Json.Encode exposing (encode, null)

    -- encode 0 null == "null"
-}
null : Value
null =
    Elm.Kernel.Json.encodeNull



-- ARRAYS


{-| Turn a `List` into a JSON array.

    import Json.Encode as Encode exposing (bool, encode, int, list, string)

    -- encode 0 (list int [1,3,4])       == "[1,3,4]"
    -- encode 0 (list bool [True,False]) == "[true,false]"
    -- encode 0 (list string ["a","b"])  == """["a","b"]"""

-}
list : (a -> Value) -> List a -> Value
list func entries =
    Elm.Kernel.Json.wrap
        (List.foldl (Elm.Kernel.Json.addEntry func) (Elm.Kernel.Json.emptyArray ()) entries)


{-| Turn an `Array` into a JSON array.
-}
array : (a -> Value) -> Array a -> Value
array func entries =
    Elm.Kernel.Json.wrap
        (Array.foldl (Elm.Kernel.Json.addEntry func) (Elm.Kernel.Json.emptyArray ()) entries)


{-| Turn an `Set` into a JSON array.
-}
set : (a -> Value) -> Set a -> Value
set func entries =
    Elm.Kernel.Json.wrap
        (Set.foldl (Elm.Kernel.Json.addEntry func) (Elm.Kernel.Json.emptyArray ()) entries)



-- OBJECTS


{-| Create a JSON object.

    import Json.Encode as Encode

    tom : Encode.Value
    tom =
        Encode.object
            [ ( "name", Encode.string "Tom" )
            , ( "age", Encode.int 42 )
            ]

    -- Encode.encode 0 tom == """{"name":"Tom","age":42}"""
-}
object : List (String, Value) -> Value
object pairs =
    Elm.Kernel.Json.wrap (
        List.foldl
            (\(k,v) obj -> Elm.Kernel.Json.addField k v obj)
            (Elm.Kernel.Json.emptyObject ())
            pairs
    )


{-| Turn a `Dict` into a JSON object.

    import Dict exposing (Dict)
    import Json.Encode as Encode

    people : Dict String Int
    people =
      Dict.fromList [ ("Tom",42), ("Sue", 38) ]

    -- Encode.encode 0 (Encode.dict identity Encode.int people)
    --   == """{"Tom":42,"Sue":38}"""
-}
dict : (k -> String) -> (v -> Value) -> Dict k v -> Value
dict toKey toValue dictionary =
    Elm.Kernel.Json.wrap (
        Dict.foldl
            (\key value obj -> Elm.Kernel.Json.addField (toKey key) (toValue value) obj)
            (Elm.Kernel.Json.emptyObject ())
            dictionary
    )
