module Views.Settings.Views exposing (view)

import Html exposing (..)
import Html.Attributes exposing (checked, class, for, id, name, type_, value)
import Html.Events exposing (onInput)
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek(..))
import Views.Settings.Types exposing (Model, SettingsMsg(..))


view : Model -> Html SettingsMsg
view model =
    div []
        [ -- First day of the week setting
          div [ class "g-0" ]
            [ label [ for "fieldset-firstDayOfWeek" ] [ text "First day of the week:" ]
            , fieldset [ id "fieldset-firstDayOfWeek" ]
                [ radio "Monday" "Monday" (model.firstDayOfWeek == Monday) UpdateFirstDayOfWeek
                , radio "Sunday" "Sunday" (model.firstDayOfWeek == Sunday) UpdateFirstDayOfWeek
                ]
            , small [ class "form-text text-muted" ]
                [ text "Note: This setting is saved in local storage of your browser" ]
            ]
        , -- Bootstrap theme setting
          div [ class "g-0 mt-3" ]
            [ label [ for "fieldset-bootstrapTheme" ] [ text "Theme:" ]
            , fieldset [ id "fieldset-bootstrapTheme" ]
                [ radio "Auto" "auto" (model.bootstrapTheme == "auto") UpdateBootstrapTheme
                , radio "Light" "light" (model.bootstrapTheme == "light") UpdateBootstrapTheme
                , radio "Dark" "dark" (model.bootstrapTheme == "dark") UpdateBootstrapTheme
                ]
            , small [ class "form-text text-muted" ]
                [ text "Choose the theme for the application. Auto follows system settings." ]
            ]
        ]


radio : String -> String -> Bool -> (String -> msg) -> Html msg
radio labelText radioValue isChecked toMsg =
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
            , name radioValue
            , checked isChecked
            , value radioValue
            , onInput toMsg
            ]
            []
        , label
            [ class "form-check-label"
            , Html.Attributes.for radioId
            ]
            [ text labelText ]
        ]
