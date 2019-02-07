module Tests exposing (..)

import Expect exposing (Expectation)
import Json.Decode as Decode exposing (Decoder, null, string)
import Json.Decode.Pipeline
    exposing
        ( optional
        , optionalAt
        , required
        , requiredAt
        , resolveResult
        )
import Test exposing (..)


{-| Run some JSON through a Decoder and return the result.
-}
runWith : String -> Decoder a -> Result String a
runWith =
    flip Decode.decodeString


isError : Result err ok -> Bool
isError result =
    case result of
        Err _ ->
            True

        Ok _ ->
            False


expectErr : Result err ok -> Expectation
expectErr result =
    isError result
        |> Expect.true ("Expected an Err but got " ++ toString result)


all : Test
all =
    describe
        "Json.Decode.Pipeline"
        [ test "should decode basic example" <|
            \() ->
                Decode.succeed (,)
                    |> required "a" string
                    |> required "b" string
                    |> runWith """{"a":"foo","b":"bar"}"""
                    |> Expect.equal (Ok ( "foo", "bar" ))
        , test "should decode requiredAt fields" <|
            \() ->
                Decode.succeed (,)
                    |> requiredAt [ "a" ] string
                    |> requiredAt [ "b", "c" ] string
                    |> runWith """{"a":"foo","b":{"c":"bar"}}"""
                    |> Expect.equal (Ok ( "foo", "bar" ))
        , test "should decode optionalAt fields" <|
            \() ->
                Decode.succeed (,)
                    |> optionalAt [ "a", "b" ] string "--"
                    |> optionalAt [ "x", "y" ] string "--"
                    |> runWith """{"a":{},"x":{"y":"bar"}}"""
                    |> Expect.equal (Ok ( "--", "bar" ))
        , test "optional succeeds if the field is not present" <|
            \() ->
                Decode.succeed (,)
                    |> optional "a" string "--"
                    |> optional "x" string "--"
                    |> runWith """{"x":"five"}"""
                    |> Expect.equal (Ok ( "--", "five" ))
        , test "optional succeeds with fallback if the field is present but null" <|
            \() ->
                Decode.succeed (,)
                    |> optional "a" string "--"
                    |> optional "x" string "--"
                    |> runWith """{"a":null,"x":"five"}"""
                    |> Expect.equal (Ok ( "--", "five" ))
        , test "optional succeeds with result of the given decoder if the field is null and the decoder decodes nulls" <|
            \() ->
                Decode.succeed (,)
                    |> optional "a" (null "null") "--"
                    |> optional "x" string "--"
                    |> runWith """{"a":null,"x":"five"}"""
                    |> Expect.equal (Ok ( "null", "five" ))
        , test "optional fails if the field is present but doesn't decode" <|
            \() ->
                Decode.succeed (,)
                    |> optional "a" string "--"
                    |> optional "x" string "--"
                    |> runWith """{"x":5}"""
                    |> expectErr
        , test "optionalAt fails if the field is present but doesn't decode" <|
            \() ->
                Decode.succeed (,)
                    |> optionalAt [ "a", "b" ] string "--"
                    |> optionalAt [ "x", "y" ] string "--"
                    |> runWith """{"a":{},"x":{"y":5}}"""
                    |> expectErr
        , test "resolveResult bubbles up decoded Err results" <|
            \() ->
                Decode.succeed Err
                    |> required "error" string
                    |> resolveResult
                    |> runWith """{"error":"invalid"}"""
                    |> expectErr
        , test "resolveResult bubbles up decoded Ok results" <|
            \() ->
                Decode.succeed Ok
                    |> required "ok" string
                    |> resolveResult
                    |> runWith """{"ok":"valid"}"""
                    |> Expect.equal (Ok "valid")
        ]
