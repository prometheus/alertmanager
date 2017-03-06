module Silences.Translator exposing (translator)

import Silences.Types exposing (Msg(..), SilencesMsg, OutMsg(NewUrl, UpdateFilter))
import Utils.Types exposing (Filter)


type alias TranslationDictionary msg =
    { onFormMsg : SilencesMsg -> msg
    , onNewUrl : String -> msg
    , onUpdateFilter : Filter -> String -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onFormMsg, onNewUrl, onUpdateFilter } msg =
    case msg of
        ForSelf internal ->
            onFormMsg internal

        ForParent (NewUrl url) ->
            onNewUrl url

        ForParent (UpdateFilter filter string) ->
            onUpdateFilter filter string
