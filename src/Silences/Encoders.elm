module Silences.Encoders exposing (..)

-- External

import Json.Encode as Encode


-- Internal

import Types exposing (Silence, Matcher)


silence : Silence -> Encode.Value
silence silence =
    Encode.object
        [ ( "createdBy", Encode.string silence.createdBy )
        , ( "comment", Encode.string silence.comment )
        , ( "startsAt", Encode.string silence.startsAt.s )
        , ( "endsAt", Encode.string silence.endsAt.s )
        , ( "matchers", (Encode.list (List.map matcher silence.matchers)) )
        ]


matcher : Matcher -> Encode.Value
matcher m =
    Encode.object
        [ ( "name", Encode.string m.name )
        , ( "value", Encode.string m.value )
        , ( "isRegex", Encode.bool m.isRegex )
        ]
