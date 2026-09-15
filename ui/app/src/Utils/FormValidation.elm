module Utils.FormValidation exposing
    ( ValidatedField
    , ValidationState(..)
    , initialField
    , updateValue
    , validate
    )


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
    }


initialField : String -> ValidatedField
initialField value =
    { value = value
    , validationState = Initial
    }


updateValue : String -> ValidatedField -> ValidatedField
updateValue value field =
    { field | value = value, validationState = Initial }


validate : (String -> Result String a) -> ValidatedField -> ValidatedField
validate validator field =
    { field | validationState = fromResult (validator field.value) }
