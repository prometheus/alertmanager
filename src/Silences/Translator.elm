module Silences.Translator exposing (translator)

import Silences.Types exposing (Msg(..), SilencesMsg, OutMsg(NewUrl))


type alias TranslationDictionary msg =
    { onFormMsg : SilencesMsg -> msg
    , onNewUrl : String -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onFormMsg, onNewUrl } msg =
    case msg of
        ForSelf internal ->
            onFormMsg internal

        ForParent (NewUrl url) ->
            onNewUrl url
