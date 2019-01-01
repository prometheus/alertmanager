{-
   Alertmanager API
   API of the Prometheus Alertmanager (https://github.com/prometheus/alertmanager)

   OpenAPI spec version: 0.0.1

   NOTE: This file is auto generated by the openapi-generator.
   https://github.com/openapitools/openapi-generator.git
   Do not edit this file manually.
-}


module Data.PostableAlert exposing (PostableAlert, decoder, encoder)

import DateTime exposing (DateTime)
import Dict exposing (Dict)
import Json.Decode as Decode exposing (Decoder)
import Json.Decode.Pipeline exposing (optional, required)
import Json.Encode as Encode


type alias PostableAlert =
    { labels : Dict String String
    , generatorURL : Maybe String
    , startsAt : Maybe DateTime
    , endsAt : Maybe DateTime
    , annotations : Maybe (Dict String String)
    }


decoder : Decoder PostableAlert
decoder =
    Decode.succeed PostableAlert
        |> required "labels" (Decode.dict Decode.string)
        |> optional "generatorURL" (Decode.nullable Decode.string) Nothing
        |> optional "startsAt" (Decode.nullable DateTime.decoder) Nothing
        |> optional "endsAt" (Decode.nullable DateTime.decoder) Nothing
        |> optional "annotations" (Decode.nullable (Decode.dict Decode.string)) Nothing


encoder : PostableAlert -> Encode.Value
encoder model =
    Encode.object
        [ ( "labels", Encode.dict identity Encode.string model.labels )
        , ( "generatorURL", Maybe.withDefault Encode.null (Maybe.map Encode.string model.generatorURL) )
        , ( "startsAt", Maybe.withDefault Encode.null (Maybe.map DateTime.encoder model.startsAt) )
        , ( "endsAt", Maybe.withDefault Encode.null (Maybe.map DateTime.encoder model.endsAt) )
        , ( "annotations", Maybe.withDefault Encode.null (Maybe.map (Encode.dict identity Encode.string) model.annotations) )
        ]
