module Silences.Encoders exposing (..)

import Json.Encode as Encode
import Silences.Types exposing (Silence)
import Utils.Types exposing (Matcher)
import Utils.Date


silence : Silence -> Encode.Value
silence silence =
    Encode.object
        [ ( "id", Encode.string silence.id )
        , ( "createdBy", Encode.string silence.createdBy )
        , ( "comment", Encode.string silence.comment )
        , ( "startsAt", Encode.string (Utils.Date.encode silence.startsAt) )
        , ( "endsAt", Encode.string (Utils.Date.encode silence.endsAt) )
        , ( "matchers", Encode.list (List.map matcher silence.matchers) )
        ]


matcher : Matcher -> Encode.Value
matcher m =
    Encode.object
        [ ( "name", Encode.string m.name )
        , ( "value", Encode.string m.value )
        , ( "isRegex", Encode.bool m.isRegex )
        ]
