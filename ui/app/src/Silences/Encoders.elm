module Silences.Encoders exposing (matcher, silence)

import Json.Encode as Encode
import Silences.Types exposing (Silence)
import Utils.Date
import Utils.Types exposing (Matcher)


silence : Silence -> Encode.Value
silence silence_ =
    Encode.object
        [ ( "id", Encode.string silence_.id )
        , ( "createdBy", Encode.string silence_.createdBy )
        , ( "comment", Encode.string silence_.comment )
        , ( "startsAt", Encode.string (Utils.Date.encode silence_.startsAt) )
        , ( "endsAt", Encode.string (Utils.Date.encode silence_.endsAt) )
        , ( "matchers", Encode.list matcher silence_.matchers )
        ]


matcher : Matcher -> Encode.Value
matcher m =
    Encode.object
        [ ( "name", Encode.string m.name )
        , ( "value", Encode.string m.value )
        , ( "isRegex", Encode.bool m.isRegex )
        ]
