module Utils.FormValidation
    exposing
        ( initialField
        , validField
        , ValidationState(..)
        , ValidatedField
        , validate
        , stringNotEmpty
        )


type ValidationState
    = Initial
    | Invalid String


type alias ValidatedField a =
    { value : String
    , validationResult : Result ValidationState a
    }


initialField : ValidatedField a
initialField =
    { value = ""
    , validationResult = Err Initial
    }


validField : a -> (a -> String) -> ValidatedField a
validField result resultToValue =
    { value = resultToValue result
    , validationResult = Ok result
    }


validate : (String -> Result String a) -> String -> ValidatedField a
validate validate input =
    { value = input
    , validationResult = validate input |> Result.mapError Invalid
    }


stringNotEmpty : String -> Result String String
stringNotEmpty string =
    if String.isEmpty string then
        Err "Should not be empty"
    else
        Ok string
