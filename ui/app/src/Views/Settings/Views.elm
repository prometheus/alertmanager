module Views.Settings.Views exposing (view)

import Html exposing (..)
import Html.Attributes exposing (checked, class, for, id, type_, value)
import Html.Events exposing (..)
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek(..))
import Views.Settings.Types exposing (Model, SettingsMsg(..))


view : Model -> Html SettingsMsg
view model =
    div []
        [ div [ class "no-gutters" ]
            [ label
                [ for "fieldset" ]
                [ text "First day of the week:" ]
            , fieldset [ id "fieldset" ]
                [ radio "Monday" (model.firstDayOfWeek == Monday) UpdateFirstDayOfWeek
                , radio "Sunday" (model.firstDayOfWeek == Sunday) UpdateFirstDayOfWeek
                ]
            , small [ class "form-text text-muted" ]
                [ text "Note: This setting is saved in local storage of your browser"
                ]
            ]
        ]


radio : String -> Bool -> (String -> msg) -> Html msg
radio radioValue isChecked msg =
    div [ class "form-check" ]
        [ input [ type_ "radio", onInput msg, class "form-check-input", id radioValue, checked isChecked, value radioValue ] []
        , label [ class "form-check-label", for radioValue ]
            [ text radioValue
            ]
        ]
