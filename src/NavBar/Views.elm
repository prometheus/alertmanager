module NavBar.Views exposing (appHeader)

import Html exposing (Html, header, text, a, nav)
import Html.Attributes exposing (class, href, title)
import Types exposing (Model, Msg)

appHeader : Model -> Html Msg
appHeader model =
    header [ class "bg-black-90 fixed w-100 ph3 pv3 pv4-ns ph4-m ph5-l" ]
        [ nav [ class "w-80 center f6 fw6 ttu tracked" ]
              [ a [ class "link dim white dib mr3", href "#", title "Home" ]
                    [ text "AlertManager" ]
              , a [ class "link dim white dib mr3", href "#/alerts", title "Alerts" ]
                  [ text "Alerts" ]
              , a [ class "link dim white dib mr3", href "#/silences", title "Silences" ]
                  [ text "Silences" ]
              , a [ class "link dim white dib", href "#/status", title "Status" ]
            [ text "Status" ]
        ]
    ]
