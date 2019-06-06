module Utils.FormValidation exposing
    ( ValidatedField
    , ValidationState(..)
    , fromResult
    , initialField
    , stringNotEmpty
    , updateValue
    , validate
    )

import List exposing (length)
import String exposing (lines)


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


type alias Config =
    { padding : Float
    , lineHeight : Float
    , minRows : Int
    , maxRows : Int
    }


config : Config
config =
    { padding = 20
    , lineHeight = 20
    , minRows = 3
    , maxRows = 15
    }


initialField : String -> ValidatedField
initialField value =
    { value = value
    , validationState = Initial
    , rows = config.minRows
    }


updateValue : String -> ValidatedField -> ValidatedField
updateValue value field =
    let
        rows =
            lines value
                |> length
                |> clamp config.minRows config.maxRows
    in
    { field | value = value, validationState = Initial, rows = rows }


validate : (String -> Result String a) -> ValidatedField -> ValidatedField
validate validator field =
    { field | validationState = fromResult (validator field.value) }


stringNotEmpty : String -> Result String String
stringNotEmpty string =
    if String.isEmpty (String.trim string) then
        Err "Should not be empty"

    else
        Ok string
