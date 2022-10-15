module Views.Settings.Views exposing (view)

import Html exposing (..)
import Html.Attributes exposing (class, for, id, selected, value)
import Html.Events exposing (..)
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek(..))
import Views.Settings.Types exposing (Model, SettingsMsg(..))


view : Model -> Html SettingsMsg
view model =
    div []
        [ div [ class "row no-gutters" ]
            [ label
                [ for "select" ]
                [ text "First day of the week:" ]
            , select
                [ onInput UpdateFirstDayOfWeek, id "select", class "form-control" ]
                [ option
                    [ value "Monday"
                    , selected
                        (model.firstDayOfWeek == Monday)
                    ]
                    [ text "Monday" ]
                , option
                    [ value "Sunday"
                    , selected
                        (model.firstDayOfWeek == Sunday)
                    ]
                    [ text "Sunday" ]
                ]
            , small [ class "form-text text-muted" ]
                [ text "Note: This setting is saved in local storage of your browser"
                ]
            ]
        ]
