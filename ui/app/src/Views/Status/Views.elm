module Views.Status.Views exposing (view)

import Html exposing (..)
import Html.Attributes exposing (class, style, classList)
import Status.Types exposing (StatusResponse, VersionInfo, ClusterStatus, ClusterPeer)
import Types exposing (Msg(MsgForStatus))
import Utils.Types exposing (ApiData(Failure, Success, Loading, Initial))
import Views.Status.Types exposing (StatusModel)
import Utils.Views


view : StatusModel -> Html Types.Msg
view { statusInfo } =
    case statusInfo of
        Success info ->
            viewStatusInfo info

        Initial ->
            Utils.Views.loading

        Loading ->
            Utils.Views.loading

        Failure msg ->
            Utils.Views.error msg


viewStatusInfo : StatusResponse -> Html Types.Msg
viewStatusInfo status =
    div []
        [ h1 [] [ text "Status" ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Uptime:" ]
            , div [ class "col-sm-10" ] [ text status.uptime ]
            ]
        , viewClusterStatus status.clusterStatus
        , viewVersionInformation status.versionInfo
        , viewConfig status.config
        ]


viewConfig : String -> Html Types.Msg
viewConfig config =
    div []
        [ h2 [] [ text "Config" ]
        , pre [ class "p-4", style [ ( "background", "#f7f7f9" ), ( "font-family", "monospace" ) ] ]
            [ code []
                [ text config
                ]
            ]
        ]


viewClusterStatus : Maybe ClusterStatus -> Html Types.Msg
viewClusterStatus clusterStatus =
    case clusterStatus of
        Just clusterStatus ->
            span []
                [ h2 [] [ text "Cluster Status" ]
                , div [ class "form-group row" ]
                    [ b [ class "col-sm-2" ] [ text "Name:" ]
                    , div [ class "col-sm-10" ] [ text clusterStatus.name ]
                    ]
                , div [ class "form-group row" ]
                    [ b [ class "col-sm-2" ] [ text "Status:" ]
                    , div [ class "col-sm-10" ]
                        [ span
                            [ classList
                                [ ( "badge", True )
                                , ( "badge-success", clusterStatus.status == "ready" )
                                , ( "badge-warning", clusterStatus.status == "settling" )
                                ]
                            ]
                            [ text clusterStatus.status ]
                        ]
                    ]
                , div [ class "form-group row" ]
                    [ b [ class "col-sm-2" ] [ text "Peers:" ]
                    , ul [ class "col-sm-10" ] <|
                        List.map viewClusterPeer clusterStatus.peers
                    ]
                ]

        Nothing ->
            span []
                [ h2 [] [ text "Mesh Status" ]
                , div [ class "form-group row" ]
                    [ div [ class "col-sm-10" ] [ text "Mesh not configured" ]
                    ]
                ]


viewClusterPeer : ClusterPeer -> Html Types.Msg
viewClusterPeer peer =
    li []
        [ div [ class "" ]
            [ b [ class "" ] [ text "Name: " ]
            , text peer.name
            ]
        , div [ class "" ]
            [ b [ class "" ] [ text "Address: " ]
            , text peer.address
            ]
        ]


viewVersionInformation : VersionInfo -> Html Types.Msg
viewVersionInformation versionInfo =
    span []
        [ h2 [] [ text "Version Information" ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Branch:" ], div [ class "col-sm-10" ] [ text versionInfo.branch ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "BuildDate:" ], div [ class "col-sm-10" ] [ text versionInfo.buildDate ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "BuildUser:" ], div [ class "col-sm-10" ] [ text versionInfo.buildUser ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "GoVersion:" ], div [ class "col-sm-10" ] [ text versionInfo.goVersion ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Revision:" ], div [ class "col-sm-10" ] [ text versionInfo.revision ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Version:" ], div [ class "col-sm-10" ] [ text versionInfo.version ] ]
        ]
