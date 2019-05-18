module Utils.FormValidation exposing
    ( ValidatedField
    , ValidationState(..)
    , fromResult
    , initialField
    , onInputWithScrollHeight
    , stringNotEmpty
    , updateTextAreaHeight
    , updateValue
    , validate
    )

import Html exposing (Html)
import Html.Events exposing (on)
import Json.Decode exposing (at, int, map)


type ValidationState
    = Initial
    | Valid
    | Invalid String


fromResult : Result String a -> ValidationState
fromResult result =
    case result of
        Ok _ ->
            Valid

        Err str ->
            Invalid str


type alias ValidatedField =
    { value : String
    , validationState : ValidationState
    , rows : Int
    }


initialField : String -> ValidatedField
initialField value =
    { value = value
    , validationState = Initial
    , rows = 3
    }


updateValue : String -> ValidatedField -> ValidatedField
updateValue value field =
    { field | value = value, validationState = Initial }


updateTextAreaHeight : Int -> ValidatedField -> ValidatedField
updateTextAreaHeight scrollHeight field =
    let
        rows =
            ceiling ((toFloat scrollHeight - 20) / 20)
    in
    { field | rows = rows }


validate : (String -> Result String a) -> ValidatedField -> ValidatedField
validate validator field =
    { field | validationState = fromResult (validator field.value) }


stringNotEmpty : String -> Result String String
stringNotEmpty string =
    if String.isEmpty (String.trim string) then
        Err "Should not be empty"

    else
        Ok string


onInputWithScrollHeight : (Int -> msg) -> Html.Attribute msg
onInputWithScrollHeight tagger =
    on "focus" (map tagger (at [ "target", "scrollHeight" ] int))
