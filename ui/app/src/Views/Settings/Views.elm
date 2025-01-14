module Views.Settings.Views exposing (view)

import Html exposing (..)
import Html.Attributes exposing (checked, class, for, id, name, type_, value)
import Html.Events exposing (onInput)
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek(..))
import Views.Settings.Types exposing (Model, SettingsMsg(..))


view : Model -> Html SettingsMsg
view model =
    div []
        [ -- Replaced "no-gutters" with either "g-0" or nothing at all,
          -- depending on if you actually need row+column layout.
          div [ class "g-0" ]
            [ label [ for "fieldset" ] [ text "First day of the week:" ]
            , fieldset [ id "fieldset" ]
                [ radio "Monday" (model.firstDayOfWeek == Monday) UpdateFirstDayOfWeek
                , radio "Sunday" (model.firstDayOfWeek == Sunday) UpdateFirstDayOfWeek
                ]
            , small [ class "form-text text-muted" ]
                [ text "Note: This setting is saved in local storage of your browser" ]
            ]
        ]


radio : String -> Bool -> (String -> msg) -> Html msg
radio radioValue isChecked toMsg =
    let
        -- Generate an ID based on the radioValue
        radioId =
            "radio-" ++ radioValue
    in
    div [ class "form-check form-check-inline ms-1 mt-1" ]
        [ input
            [ type_ "radio"
            , class "form-check-input"
            , id radioId
            , name "firstDayOfWeek"
            , checked isChecked
            , value radioValue
            , onInput toMsg
            ]
            []
        , label
            [ class "form-check-label"
            , Html.Attributes.for radioId
            ]
            [ text radioValue ]
        ]
