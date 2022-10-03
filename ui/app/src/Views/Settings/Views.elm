module Views.Settings.Views exposing (view)

import Html exposing (..)
import Html.Attributes exposing (class, for, id, selected, value)
import Html.Events exposing (..)
import Views.Settings.Types exposing (Model, SettingsMsg(..))


view : Model -> Html SettingsMsg
view model =
    div []
        [ h1 []
            [ text "Settings" ]
        , div [ class "form-group" ]
            [ label
                [ for "select" ]
                [ text "Start of Week:" ]
            , select
                [ onInput UpdateStartWeekAtMonday, id "select", class "form-control" ]
                [ option
                    [ value "1"
                    , selected
                        (model.startOfWeek == 1)
                    ]
                    [ text "Monday" ]
                , option
                    [ value "7"
                    , selected
                        (model.startOfWeek == 7)
                    ]
                    [ text "Sunday" ]
                ]
            , small [ class "form-text text-muted" ]
                [ text "Note: This setting is saved in local storage of your browser"
                ]
            ]
        ]
