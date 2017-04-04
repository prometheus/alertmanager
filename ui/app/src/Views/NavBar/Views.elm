module Views.NavBar.Views exposing (appHeader)

import Html exposing (Html, header, text, a, nav, ul, li)
import Html.Attributes exposing (class, href, title, style)


appHeader : List ( String, String ) -> Html msg
appHeader links =
    let
        headerLinks =
            List.map (\( link, text ) -> headerLink link text) links
    in
        header
            [ class "navbar navbar-toggleable-md navbar-light "
            , style [ ( "margin-bottom", "3rem" ), ( "border-bottom", "1px solid rgba(0,0,0,.15)" ) ]
            ]
            [ nav [ class "container" ]
                [ a [ class "navbar-brand", href "#" ] [ text "AlertManager" ]
                , ul [ class "navbar-nav" ] headerLinks
                ]
            ]


headerLink : String -> String -> Html msg
headerLink link linkText =
    li [ class "nav-item" ]
        [ a [ class "nav-link", href link, title linkText ]
            [ text linkText ]
        ]
